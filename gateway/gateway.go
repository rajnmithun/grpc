package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	_ "github.com/joho/godotenv/autoload"
	context "golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	things "github.com/aodin/grpc/go"
	"github.com/aodin/grpc/server"
)

var (
	Addr = "localhost:8080"
)

// newGateway returns a new gateway server which translates HTTP into gRPC.
func newGateway(ctx context.Context, opts ...runtime.ServeMuxOption) (http.Handler, error) {
	mux := runtime.NewServeMux(opts...)

	creds, err := credentials.NewClientTLSFromFile(server.GetCertFile(), "")
	if err != nil {
		return mux, fmt.Errorf("TLS creation failed: %v", err)
	}

	// If using TLS
	dialOpts := []grpc.DialOption{grpc.WithTransportCredentials(creds)}

	// If not using TLS
	// dialOpts := []grpc.DialOption{grpc.WithInsecure()}

	if err := things.RegisterThingsHandlerFromEndpoint(ctx, mux, server.Addr, dialOpts); err != nil {
		return nil, err
	}
	return mux, nil
}

// handleCORS allows Cross Origin Resoruce Sharing from any origin.
// Don't do this without consideration in production systems.
func handleCORS(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			if r.Method == "OPTIONS" && r.Header.Get("Access-Control-Request-Method") != "" {
				preflightHandler(w, r)
				return
			}
		}
		h.ServeHTTP(w, r)
	})
}

// loggingResponseWriter records the status code of the response
type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func NewLoggingResponseWriter(w http.ResponseWriter) *loggingResponseWriter {
	return &loggingResponseWriter{w, http.StatusOK}
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}

func logRequestWrapper(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Wrap the response writer in order to save status code
		writer := NewLoggingResponseWriter(w)
		start := time.Now()
		h.ServeHTTP(writer, r) // The original request is evaluated here
		elapsed := time.Now().Sub(start).Seconds()
		log.Printf(
			"%d %s %s %.6f",
			writer.statusCode, r.Method, r.URL, elapsed,
		)
	})
}

// headers is allowed CORS headers
var headers = []string{
	"Accept",
	"Accept-Encoding",
	"Authorization",
	"Content-Type",
	"Origin",
	"User-Agent",
	"X-CSRF-Token",
	"X-CSRFToken",
	"X-Request-ID",
	"X-Requested-With",
}

// methods are the HTTP methods allowed by CORS
var methods = []string{
	http.MethodGet,
	http.MethodHead,
	http.MethodPost,
	http.MethodPut,
	http.MethodPatch,
	http.MethodDelete,
}

// preflightHandler sets headers values for CORS and then short-circuits
// the request
func preflightHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Headers", strings.Join(headers, ","))
	w.Header().Set("Access-Control-Allow-Methods", strings.Join(methods, ","))
	w.Header().Set("Access-Control-Max-Age", "3600") // One hour
	return
}

// Run starts a HTTP server and blocks forever if successful.
func New(address string, opts ...runtime.ServeMuxOption) error {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	mux := http.NewServeMux()
	gw, err := newGateway(ctx, opts...)
	if err != nil {
		return err
	}
	mux.Handle("/", gw)
	log.Printf("starting gateway server on %s\n", Addr)

	return http.ListenAndServe(
		address,
		logRequestWrapper(handleCORS(mux)),
	)
}

func main() {
	log.SetFlags(0) // Remove timestamp - it will be set by log ingestion
	if err := New(Addr); err != nil {
		log.Fatal(err)
	}
}
