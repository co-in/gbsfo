package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"flag"
	"fmt"
	api "github.com/co-in/gbsfo-test/pkg/api/v1"
	service "github.com/co-in/gbsfo-test/pkg/service/v1"
	_ "github.com/mattn/go-sqlite3"
	"google.golang.org/grpc"
	"log"
	"net"
	"os"
	"os/signal"
)

func readJwtSecret(jwtSecretFile string) ([]byte, error) {
	f, err := os.OpenFile(jwtSecretFile, os.O_RDWR|os.O_CREATE, 0755)
	if err != nil {
		return nil, fmt.Errorf("cant open jwt file: %v", err)
	}

	defer func() {
		_ = f.Close()
	}()

	var buff = make([]byte, 32)

	info, err := f.Stat()
	if info.Size() == 0 {
		_, err = rand.Read(buff)
		if err != nil {
			return nil, fmt.Errorf("cant generate jwt vector: %v", err)
		}

		_, err = f.Write(buff)
		if err != nil {
			return nil, fmt.Errorf("cant generate jwt vector: %v", err)
		}

		return buff, nil
	}

	_, err = f.Read(buff)
	if err != nil {
		return nil, fmt.Errorf("cant read jwt file: %v", err)
	}

	return buff, nil
}

func main() {
	port := flag.String("port", ":12000", "gRPC port to bind")
	dbFile := flag.String("db-file", "users.db", "SQLite3 file location")
	jwtSecretFile := flag.String("jwtSecretFile", "secret.dat", "JWT Secret file location")

	flag.Parse()

	jwtSecret, err := readJwtSecret(*jwtSecretFile)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}

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
	api.RegisterAuthServer(server, service.NewAuthServiceServer(jwtSecret, db))

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for range c {
			log.Println("shutting down gRPC auth...")
			server.GracefulStop()
			<-ctx.Done()
		}
	}()

	log.Println("starting gRPC auth...")

	err = server.Serve(listen)
	if err != nil {
		log.Fatalf("failed auth serve: %v", err)
	}
}
