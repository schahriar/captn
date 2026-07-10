package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"

	"github.com/schahriar/captn/pkg/mcp"
	"github.com/schahriar/captn/pkg/tui"
)

const Port = 22786

const taskPath = "/task"

type Server struct {
	srv *http.Server
	ln  net.Listener
}

// Acts as a repeater of CLI commands since Claude spends less tokens / time on deciding on a CLI call vs MCP tool call
func NewServer() *Server {
	mux := http.NewServeMux()
	registerTask(mux, mcp.NewSearchAndExplainTool())
	registerTask(mux, mcp.NewExplainTool())

	// banstructlit:ignore
	return &Server{srv: &http.Server{Handler: mux}}
}

func (s *Server) Listen() error {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", Port))
	if err != nil {
		return fmt.Errorf("port %d is already in use: %w", Port, err)
	}

	s.ln = ln
	return nil
}

// Serve starts handling requests in the background. Request contexts derive
// from ctx, so values like the TUI status provider reach the tool calls.
func (s *Server) Serve(ctx context.Context) {
	s.srv.BaseContext = func(net.Listener) context.Context { return ctx }

	go func() {
		_ = s.srv.Serve(s.ln)
	}()
}

func (s *Server) Close() error {
	return s.srv.Close()
}

func registerTask[In any, Out any](mux *http.ServeMux, tool mcp.Tool[In, Out]) {
	mux.HandleFunc(taskPath+"/"+tool.Name(), handleTask(tool))
}

func handleTask[In any, Out any](tool mcp.Tool[In, Out]) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var input In
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, fmt.Sprintf("invalid task payload: %v", err), http.StatusBadRequest)
			return
		}

		ctx := r.Context()
		if provider, ok := tui.GetStatusProvider(ctx); ok {
			done := provider.PushTask(tui.StatusTypeProgress, fmt.Sprintf("-ing (%s) for ", tool.Name()))
			defer done()
		}

		_, out, err := tool.Call(ctx, nil, input)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	}
}
