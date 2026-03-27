package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/cloudflare/tableflip"
)

// version is set at build time via -ldflags="-X main.version=vX.Y.Z".
// It defaults to "dev" for local builds.
var version = "dev"

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "version" || os.Args[1] == "--version" || os.Args[1] == "-version") {
		fmt.Println("orbitor", version)
		return
	}

	if len(os.Args) < 2 || os.Args[1] == "tui" {
		// Default: TUI client
		runTUIMode(false)
		return
	}

	switch os.Args[1] {
	case "new":
		runTUIMode(true)
	case "server":
		runServerMode()
	case "setup":
		if err := runSetup(); err != nil {
			log.Fatal(err)
		}
	case "setup-terminal":
		runTerminalSetup()
	case "service":
		runServiceSubcommand()
	case "procmanager":
		clientCfg := LoadClientConfig()
		dir, err := OrbitorDir()
		if err != nil {
			log.Fatalf("orbitor dir: %v", err)
		}
		dbPath := filepath.Join(dir, "orbitor.db")
		store, err := NewStore(dbPath)
		if err != nil {
			log.Fatalf("open store: %v", err)
		}
		defer store.Close()
		_ = clientCfg
		pm := NewProcManager(store)
		if err := pm.StartServer("127.0.0.1:19101"); err != nil {
			log.Fatalf("procmanager failed: %v", err)
		}
	default:
		if strings.HasPrefix(os.Args[1], "-") {
			// Legacy flag-style invocation: fall through to TUI for compatibility.
			runTUIMode(false)
		} else {
			printUsage()
		}
	}
}

// runTUIMode launches the TUI client. If newSession is true it creates a new
// session for the current working directory (equivalent to the old -tui -new flags).
func runTUIMode(newSession bool) {
	clientCfg := LoadClientConfig()
	serverURL := clientCfg.ServerURL

	if newSession {
		// Positional args after "new": [backend [model]]
		args := os.Args[2:]
		backend := clientCfg.DefaultBackend
		model := clientCfg.DefaultModel
		skip := clientCfg.SkipPermissions
		plan := clientCfg.PlanMode
		if len(args) >= 1 {
			backend = args[0]
		}
		if len(args) >= 2 {
			model = args[1]
		}
		if err := RunTUI(serverURL, true, backend, model, skip, plan); err != nil {
			log.Fatalf("tui: %v", err)
		}
		closeLocalSTTModel()
		hardExit()
		return
	}

	if err := RunTUI(serverURL, false, "", "", false, false); err != nil {
		log.Fatalf("tui: %v", err)
	}
	closeLocalSTTModel()
	hardExit()
}

// runServerMode starts the HTTP server in the foreground with tableflip support.
func runServerMode() {
	clientCfg := LoadClientConfig()
	addr := clientCfg.ListenAddr
	if addr == "" {
		addr = "127.0.0.1:8080"
	}

	// Allow --addr flag override when running server subcommand.
	args := os.Args[2:]
	for i, a := range args {
		if a == "--addr" && i+1 < len(args) {
			addr = args[i+1]
		} else if strings.HasPrefix(a, "--addr=") {
			addr = strings.TrimPrefix(a, "--addr=")
		}
	}

	// Print binding hint.
	if strings.HasPrefix(addr, "100.") {
		log.Printf("Binding to Tailscale address: %s", addr)
	} else if strings.HasPrefix(addr, "127.") {
		log.Printf("Binding to localhost only. Use 'orbitor setup' to configure Tailscale access.")
	}

	// Ensure ~/.orbitor/ exists and open the database there.
	dir, err := OrbitorDir()
	if err != nil {
		log.Fatalf("orbitor dir: %v", err)
	}
	dbPath := filepath.Join(dir, "orbitor.db")
	store, err := NewStore(dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer store.Close()

	// Set up tableflip for graceful binary upgrades.
	// On SIGHUP the current process spawns the new binary, hands off the
	// listening socket, then drains and exits. Clients reconnect automatically.
	upg, err := tableflip.New(tableflip.Options{
		UpgradeTimeout: 30 * time.Second,
	})
	if err != nil {
		log.Fatalf("tableflip: %v", err)
	}

	// SIGHUP triggers a graceful upgrade (new binary inherits the listener).
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGHUP)
		for range sig {
			log.Println("received SIGHUP, triggering upgrade...")
			if err := upg.Upgrade(); err != nil {
				log.Printf("upgrade failed: %v", err)
			}
		}
	}()

	// SIGINT/SIGTERM trigger a clean shutdown (no upgrade).
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		log.Println("received shutdown signal")
		upg.Stop()
	}()

	// Get listener from tableflip (inherited from parent process during
	// upgrade, or freshly created on first start).
	ln, err := upg.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}

	// Load optional config (WhatsApp settings, Ollama summarizer, etc.)
	if _, err := LoadConfig("config/config.yaml"); err != nil {
		log.Printf("config: %v (optional features need config/config.yaml)", err)
	}

	// Create LLM summarizer. Uses llamafile by default (downloads binary and model
	// on first use to CacheDir). Set ollama.url in config to use Ollama instead.
	sumCfg := SummarizerConfig{}
	if AppConfig != nil {
		sumCfg.OllamaURL = AppConfig.Ollama.URL
		sumCfg.OllamaModel = AppConfig.Ollama.Model
		sumCfg.CacheDir = AppConfig.LLM.CacheDir
		sumCfg.ModelURL = AppConfig.LLM.ModelURL
	}
	summarizer := NewSummarizer(sumCfg)
	// Pre-warm summarizer (best-effort) so the first notification is faster.
	go func() {
		if summarizer == nil {
			return
		}
		if api, model, err := summarizer.ensureServer(); err != nil {
			log.Printf("summarizer prewarm failed: %v", err)
		} else {
			log.Printf("summarizer prewarm ready: %s model=%s", api, model)
		}
	}()

	// Load persisted sessions (hubs start immediately for WS history).
	// Backend processes are NOT spawned yet — we need the old process to
	// exit first so its copilot processes release their ports.
	sm := NewSessionManager(store, summarizer)

	// Initialize WhatsApp client
	waDBPath := "whatsmeow.db"
	if AppConfig != nil && AppConfig.WhatsApp.DBPath != "" {
		waDBPath = AppConfig.WhatsApp.DBPath
	}
	if wac, err := NewWAClient(waDBPath); err != nil {
		log.Printf("whatsapp init: %v (whatsapp features disabled)", err)
	} else {
		waClient = wac
		if wac.IsPaired() {
			if err := wac.Connect(context.Background()); err != nil {
				log.Printf("whatsapp connect: %v", err)
			} else {
				log.Printf("whatsapp: connected as %s", wac.PhoneNumber())
			}
		} else {
			log.Println("whatsapp: not paired — use POST /api/whatsapp/pair to link")
		}
	}

	// Initialize FCM sender for push notifications (requires Firebase service account).
	var fcmSender *FCMSender
	if AppConfig != nil && AppConfig.Firebase.ServiceAccountPath != "" {
		fcmSender = NewFCMSender(AppConfig.Firebase.ServiceAccountPath, store, summarizer)
	}

	h := NewHandlers(sm, fcmSender)
	sm.fcm = fcmSender
	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("GET /api/sessions", h.ListSessions)
	mux.HandleFunc("POST /api/sessions", h.CreateSession)
	mux.HandleFunc("POST /api/sessions/{id}/clone-prompt", h.CloneSessionAndPrompt)
	mux.HandleFunc("DELETE /api/sessions/{id}", h.DeleteSession)
	mux.HandleFunc("PATCH /api/sessions/{id}", h.UpdateSession)
	mux.HandleFunc("POST /api/sessions/{id}/kill", h.KillSession)
	mux.HandleFunc("POST /api/sessions/{id}/revive", h.ReviveSession)
	mux.HandleFunc("GET /ws/sessions/{id}", h.SessionWebSocket)
	mux.HandleFunc("GET /ws/events", h.EventsWebSocket)
	mux.HandleFunc("GET /api/notifications", h.PollNotifications)
	mux.HandleFunc("POST /api/device-token", h.RegisterDeviceToken)
	mux.HandleFunc("GET /api/browse", h.BrowseDir)
	mux.HandleFunc("GET /api/mission-summary", h.MissionSummary)
	mux.HandleFunc("POST /api/self-update", h.SelfUpdate)
	mux.HandleFunc("POST /api/send-apk", h.SendAPK)
	mux.HandleFunc("POST /api/release-apk", h.SendAPK)
	// Literal path alias so plain POST /api/release-apk works with default ServeMux
	mux.HandleFunc("/api/release-apk", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.Header().Set("Allow", "POST")
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		h.SendAPK(w, r)
	})
	mux.HandleFunc("POST /api/enhance-prompt", h.EnhancePrompt)
	mux.HandleFunc("GET /api/sessions/{id}/debrief", h.SessionDebrief)
	mux.HandleFunc("GET /api/sessions/{id}/suggestions", h.SessionSuggestions)
	mux.HandleFunc("GET /api/whatsapp/status", h.WhatsAppStatus)
	mux.HandleFunc("POST /api/whatsapp/pair", h.WhatsAppPair)
	mux.HandleFunc("GET /api/whatsapp/qr", h.WhatsAppQR)
	mux.HandleFunc("POST /api/whatsapp/logout", h.WhatsAppLogout)

	// Serve Flutter web build as fallback
	webDir := "mobile/build/web"
	if info, err := os.Stat(webDir); err == nil && info.IsDir() {
		// Register root file server using path-only pattern to avoid conflicts
		mux.Handle("/", http.FileServer(http.Dir(webDir)))
	} else {
		// Fallback HTML page for when web build is not present
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<!DOCTYPE html><html><body style="background:#1a1a2e;color:#eee;font-family:monospace;padding:2em">
<h1>orbitor</h1><p>Server running. Build the Flutter app with <code>cd mobile && flutter build web</code> to serve the UI here.</p>
<p>API: <a href="/api/sessions" style="color:#0ff">/api/sessions</a></p></body></html>`))
		})
	}

	srv := &http.Server{Handler: corsMiddleware(mux)}

	go func() {
		log.Printf("orbitor listening on %s (pid %d)", addr, os.Getpid())
		if err := srv.Serve(ln); err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	// Signal to tableflip that we're ready to accept connections.
	// During an upgrade this tells the old process it can start draining.
	if err := upg.Ready(); err != nil {
		log.Fatal(err)
	}

	// If this is the child process in an upgrade, wait for the parent to
	// fully exit before respawning backends. This ensures the old copilot
	// processes have released their ports.
	if upg.HasParent() {
		log.Println("waiting for previous process to exit...")
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		if err := upg.WaitForParent(ctx); err != nil {
			log.Printf("wait for parent: %v (proceeding anyway)", err)
		}
		cancel()
	}

	// Now it's safe to spawn backend processes.
	sm.RespawnSessions()

	// Block until tableflip tells us to exit (upgrade complete or shutdown signal).
	<-upg.Exit()

	log.Println("shutting down...")
	summarizer.Stop()
	sm.Shutdown()
	if waClient != nil {
		waClient.Disconnect()
	}

	// Gracefully shut down the HTTP server. The 5s timeout ensures
	// long-lived WebSocket connections are force-closed so the old
	// process exits promptly during upgrades.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)

	// Free the whisper model after all sessions are stopped so that no
	// session can run a final inference on a freed context.
	closeLocalSTTModel()

	log.Println("bye")

	// Use _exit(2) to skip C++ static destructors. The ggml-metal backend
	// stores Metal devices in a static vector whose destructor asserts that
	// all GPU resource sets are empty. Even after whisper_free, the
	// destructor ordering during __cxa_finalize is unreliable. Calling
	// _exit avoids this class of GPU-backend teardown crashes entirely.
	hardExit()
}

// runServiceSubcommand dispatches service management commands.
func runServiceSubcommand() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: orbitor service [install|uninstall|start|stop|restart|status|logs]")
		return
	}
	switch os.Args[2] {
	case "install":
		ServiceInstall()
	case "uninstall":
		ServiceUninstall()
	case "start":
		ServiceStart()
	case "stop":
		ServiceStop()
	case "restart":
		ServiceRestart()
	case "status":
		ServiceStatus()
	case "logs":
		ServiceLogs()
	default:
		fmt.Printf("Unknown service command: %s\n", os.Args[2])
	}
}

// printUsage prints CLI usage information.
func printUsage() {
	fmt.Println(`orbitor - AI coding assistant bridge

Usage:
  orbitor              Open TUI client (default)
  orbitor new          Create new session for current directory
  orbitor server       Run server in foreground
  orbitor setup        Run setup wizard
  orbitor service      Manage background service
    install            Install and start as system service
    uninstall          Stop and remove service
    start              Start service
    stop               Stop service
    restart            Restart service
    status             Show service status
    logs               Tail service logs`)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
