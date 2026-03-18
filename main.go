package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cloudflare/tableflip"
)

func main() {
	addr := flag.String("addr", "0.0.0.0:8080", "listen address")
	tuiMode := flag.Bool("tui", false, "run terminal UI client")
	serverURL := flag.String("server", "http://127.0.0.1:8080", "bridge server URL used in -tui mode")
	procOnly := flag.Bool("procmanager", false, "run procmanager only")
	newMode := flag.Bool("new", false, "create a new session for cwd and attach in tui")
	flag.Parse()

	if *tuiMode {
		// Determine backend/model from remaining positional args when -new is used
		if *newMode {
			args := flag.Args()
			backend := "copilot"
			model := ""
			if len(args) >= 1 {
				backend = args[0]
			}
			if len(args) >= 2 {
				model = args[1]
			}
			if err := RunTUI(*serverURL, true, backend, model, true); err != nil {
				log.Fatalf("tui: %v", err)
			}
			return
		}

		if err := RunTUI(*serverURL, false, "", "", false); err != nil {
			log.Fatalf("tui: %v", err)
		}
		return
	}

	store, err := NewStore("copilot-bridge.db")
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer store.Close()

	if *procOnly {
		pm := NewProcManager(store)
		if err := pm.StartServer("127.0.0.1:19101"); err != nil {
			log.Fatalf("procmanager failed: %v", err)
		}
		return
	}

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
	ln, err := upg.Listen("tcp", *addr)
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

	h := NewHandlers(sm)
	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("GET /api/sessions", h.ListSessions)
	mux.HandleFunc("POST /api/sessions", h.CreateSession)
	mux.HandleFunc("DELETE /api/sessions/{id}", h.DeleteSession)
	mux.HandleFunc("PATCH /api/sessions/{id}", h.UpdateSession)
	mux.HandleFunc("GET /ws/sessions/{id}", h.SessionWebSocket)
	mux.HandleFunc("GET /ws/events", h.EventsWebSocket)
	mux.HandleFunc("GET /api/notifications", h.PollNotifications)
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
	mux.HandleFunc("GET /api/whatsapp/status", h.WhatsAppStatus)
	mux.HandleFunc("POST /api/whatsapp/pair", h.WhatsAppPair)
	mux.HandleFunc("GET /api/whatsapp/qr", h.WhatsAppQR)
	mux.HandleFunc("POST /api/whatsapp/logout", h.WhatsAppLogout)

	// Serve Flutter web build as fallback
	webDir := "mobile/build/web"
	if info, err := os.Stat(webDir); err == nil && info.IsDir() {
		mux.Handle("GET /", http.FileServer(http.Dir(webDir)))
	} else {
		mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<!DOCTYPE html><html><body style="background:#1a1a2e;color:#eee;font-family:monospace;padding:2em">
<h1>copilot-bridge</h1><p>Server running. Build the Flutter app with <code>cd mobile && flutter build web</code> to serve the UI here.</p>
<p>API: <a href="/api/sessions" style="color:#0ff">/api/sessions</a></p></body></html>`))
		})
	}

	srv := &http.Server{Handler: corsMiddleware(mux)}

	go func() {
		log.Printf("copilot-bridge listening on %s (pid %d)", *addr, os.Getpid())
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

	log.Println("bye")
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
