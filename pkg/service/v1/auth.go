package v1

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/golang-jwt/jwt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/co-in/gbsfo-test/pkg/api/v1"
)

type authServiceServer struct {
	jwtSecret []byte
	db        *sql.DB
}

func (s *authServiceServer) CheckJWTToken(ctx context.Context, request *v1.CheckJwtTokenRequest) (*v1.CheckJwtTokenResponse, error) {
	if request.Token == "" {
		return &v1.CheckJwtTokenResponse{Success: false}, status.Error(codes.InvalidArgument, "empty token")
	}

	token, err := jwt.Parse(request.Token, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		return s.jwtSecret, nil
	})

	if err != nil {
		return &v1.CheckJwtTokenResponse{Success: false}, err
	}

	return &v1.CheckJwtTokenResponse{Success: token.Valid}, nil
}

func NewAuthServiceServer(jwtSecret []byte, db *sql.DB) v1.AuthServer {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err := db.ExecContext(ctx, `
	CREATE TABLE IF NOT EXISTS user (
		id INTEGER PRIMARY KEY,
		login VARCHAR(255) UNIQUE,
		password_hash CHARACTER(64)
	);`)

	if err != nil {
		log.Print(err)
	}

	return &authServiceServer{
		jwtSecret: jwtSecret,
		db:        db,
	}
}

func (s *authServiceServer) hash(pass string) string {
	hash := sha256.Sum256([]byte(pass))

	return fmt.Sprintf("%064X", hash[:])
}

func (s *authServiceServer) connect(ctx context.Context) (*sql.Conn, error) {
	c, err := s.db.Conn(ctx)
	if err != nil {
		return nil, status.Error(codes.Unknown, "failed to connect to database-> "+err.Error())
	}

	return c, nil
}

func (s *authServiceServer) SignUp(ctx context.Context, req *v1.SignUpRequest) (*v1.SignUpResponse, error) {
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO `user`(ROWID, `login`, `password_hash`) VALUES(null, ?, ?)",
		req.Login, s.hash(req.Pass),
	)
	if err != nil {
		return nil, status.Error(codes.Unknown, "failed to insert into `user` "+err.Error())
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, status.Error(codes.Unknown, "failed to retrieve id for created ToDo-> "+err.Error())
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"id":    id,
		"login": req.Login,
	})

	tokenString, err := token.SignedString(s.jwtSecret)

	return &v1.SignUpResponse{
		Token: tokenString,
	}, nil
}

func (s *authServiceServer) Login(ctx context.Context, req *v1.LoginRequest) (*v1.LoginResponse, error) {
	row := s.db.QueryRowContext(ctx, "SELECT id FROM `user` WHERE login = ? AND password_hash = ?",
		req.Login, s.hash(req.Pass))

	var id int64
	err := row.Scan(&id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, status.Error(codes.NotFound, "user not found")
		}

		return nil, status.Error(codes.Unknown, "failed to select into `user` "+err.Error())
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"id":    id,
		"login": req.Login,
	})

	tokenString, err := token.SignedString(s.jwtSecret)

	return &v1.LoginResponse{
		Token: tokenString,
	}, nil
}
