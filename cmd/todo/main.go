package main

import (
	"context"
	"database/sql"
	"flag"
	api "github.com/co-in/gbsfo-test/pkg/api/v1"
	service "github.com/co-in/gbsfo-test/pkg/service/v1"
	_ "github.com/mattn/go-sqlite3"
	"google.golang.org/grpc"
	"log"
	"net"
	"os"
	"os/signal"
)

func main() {
	port := flag.String("port", ":13000", "gRPC port to bind")
	dbFile := flag.String("db-file", "todo.db", "SQLite3 file location")

	flag.Parse()

	db, err := sql.Open("sqlite3", *dbFile)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	listen, err := net.Listen("tcp", *port)
	if err != nil {
		log.Fatalf("failed tcp listen: %v", err)
	}

	server := grpc.NewServer()
	ctx := context.Background()
	api.RegisterTodoServer(server, service.NewTodoServiceServer(db))

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for range c {
			log.Println("shutting down gRPC todo...")
			server.GracefulStop()
			<-ctx.Done()
		}
	}()

	log.Println("starting gRPC todo...")

	err = server.Serve(listen)
	if err != nil {
		log.Fatalf("failed todo serve: %v", err)
	}
}
