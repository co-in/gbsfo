package main

import (
	"context"
	"flag"
	"fmt"
	v1 "github.com/co-in/gbsfo-test/pkg/api/v1"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"log"
	"net/http"
	"os"
	"os/signal"
)

var authClient v1.AuthClient

func AccessLogInterceptorStream(
	ctx context.Context,
	desc *grpc.StreamDesc,
	cc *grpc.ClientConn,
	method string,
	streamer grpc.Streamer,
	opts ...grpc.CallOption,
) (grpc.ClientStream, error) {
	return nil, nil
}

func AccessLogInterceptorUnary(
	ctx context.Context,
	method string,
	req interface{},
	reply interface{},
	cc *grpc.ClientConn,
	invoker grpc.UnaryInvoker,
	opts ...grpc.CallOption,
) error {
	md, _ := metadata.FromOutgoingContext(ctx)

	if len(md["authorization"]) == 0 {
		return status.Error(codes.PermissionDenied, "empty auth header")
	}

	tokenString := md["authorization"][0]
	if tokenString == "" {
		return status.Error(codes.PermissionDenied, "empty token")
	}

	callContext := context.Background()

	resp, err := authClient.CheckJWTToken(callContext, &v1.CheckJwtTokenRequest{Token: tokenString})
	if err != nil {
		return fmt.Errorf("JWT token check: %v", err)
	}

	if !resp.Success {
		return status.Error(codes.PermissionDenied, "invalid token")
	}

	return invoker(callContext, method, req, reply, cc, opts...)
}

func main() {
	var gRPCPortAuth = flag.String("grpc-port-auth", ":12000", "gRPC port to bind")
	var gRPCPortTodo = flag.String("grpc-port-todo", ":13000", "gRPC port to bind")
	var HTTPPort = flag.String("http-port", ":8080", "gRPC port to bind")

	flag.Parse()

	ctx := context.Background()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	mux := runtime.NewServeMux(
		runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{
			OrigName:     true,
			EmitDefaults: true,
		}),
	)

	var err error

	conn, err := grpc.Dial(*gRPCPortAuth, grpc.WithInsecure())
	if err != nil {
		log.Fatalf("get Auth client: %v", err)
	}
	defer conn.Close()
	authClient = v1.NewAuthClient(conn)

	err = v1.RegisterAuthHandlerFromEndpoint(ctx, mux, *gRPCPortAuth, []grpc.DialOption{
		grpc.WithInsecure(),
	})
	if err != nil {
		log.Fatalf("register Auth handler: %v", err)
	}

	err = v1.RegisterTodoHandlerFromEndpoint(ctx, mux, *gRPCPortTodo, []grpc.DialOption{
		grpc.WithInsecure(),
		grpc.WithUnaryInterceptor(AccessLogInterceptorUnary),
		//grpc.WithStreamInterceptor(AccessLogInterceptorStream),
	})
	if err != nil {
		log.Fatalf("register Todo handler: %v", err)
	}

	srv := &http.Server{
		Addr:    *HTTPPort,
		Handler: mux,
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for range c {
			_ = srv.Shutdown(ctx)
		}
	}()

	log.Println("starting HTTP gateway...")

	err = srv.ListenAndServe()
	if err != nil {
		log.Fatalf("serve HTTP server: %v", err)
	}
}
