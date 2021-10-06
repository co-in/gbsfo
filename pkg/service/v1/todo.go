package v1

import (
	"context"
	"database/sql"
	"fmt"
	v1 "github.com/co-in/gbsfo-test/pkg/api/v1"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"log"
	"time"
)

type todoServiceServer struct {
	db *sql.DB
}

func NewTodoServiceServer(db *sql.DB) v1.TodoServer {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err := db.ExecContext(ctx, `
	CREATE TABLE IF NOT EXISTS task (
		id INTEGER PRIMARY KEY,
		status INTEGER,
		description TEXT
	);`)

	if err != nil {
		log.Print(err)
	}

	return &todoServiceServer{
		db: db,
	}
}

func (s *todoServiceServer) connect(ctx context.Context) (*sql.Conn, error) {
	c, err := s.db.Conn(ctx)
	if err != nil {
		return nil, status.Error(codes.Unknown, "failed to connect to database-> "+err.Error())
	}

	return c, nil
}

func (s *todoServiceServer) countTaskRecord(ctx context.Context) (int, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT count(*) AS total FROM `task`")
	if err != nil {
		return 0, fmt.Errorf("failed to NamedQuery: %w", err)
	}
	defer rows.Close()

	rows.Next()

	var count int

	if err := rows.Scan(&count); err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}

		return 0, fmt.Errorf("failed to Scan: %w", err)
	}

	return count, nil
}

func (s *todoServiceServer) insert(ctx context.Context, task *v1.Task) (int64, error) {
	res, err := s.db.ExecContext(ctx, "INSERT INTO `task` (ROWID, `status`, `description`) VALUES (null, ?, ?)",
		task.Status, task.Description)
	if err != nil {
		return 0, fmt.Errorf("insert: %v", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("getting id: %v", err)
	}

	return id, err
}

func (s *todoServiceServer) searchTaskRecord(ctx context.Context, limit, offset int) ([]*v1.Task, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, status, description FROM `task` LIMIT ? OFFSET ?", limit, offset)
	if err != nil {
		return nil, fmt.Errorf("search task: %v", err)
	}
	defer rows.Close()
	tasks := make([]*v1.Task, 0)

	for rows.Next() {
		var task v1.Task

		if err = rows.Scan(&task.Id, &task.Status, &task.Description); err != nil {
			return nil, fmt.Errorf("search task scan: %v", err)
		}

		tasks = append(tasks, &task)
	}

	return tasks, nil
}

func (s *todoServiceServer) getTaskById(ctx context.Context, id int64) (*v1.Task, error) {
	row := s.db.QueryRowContext(ctx, "SELECT id, status, description FROM `task` WHERE id = ?", id)
	var task = new(v1.Task)
	err := row.Scan(&task.Id, &task.Status, &task.Description)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, status.Error(codes.NotFound, "task not found")
		}

		return nil, fmt.Errorf("scan task by id#%d: %v", id, err)
	}

	return task, nil

}

func (s *todoServiceServer) CreateTask(ctx context.Context, request *v1.CreateTaskRequest) (*v1.CreateTaskResponse, error) {
	id, err := s.insert(ctx, request.Task)
	if err != nil {
		return nil, fmt.Errorf("create task: %v", err)
	}

	task, err := s.getTaskById(ctx, id)

	return &v1.CreateTaskResponse{
		Task: task,
	}, err
}

func (s *todoServiceServer) ReadTask(ctx context.Context, request *v1.ReadTaskRequest) (*v1.ReadTaskResponse, error) {
	task, err := s.getTaskById(ctx, request.Id)
	if err != nil {
		return nil, err
	}

	return &v1.ReadTaskResponse{Task: task}, nil
}

func (s *todoServiceServer) UpdateTask(ctx context.Context, request *v1.UpdateTaskRequest) (*v1.UpdateTaskResponse, error) {
	_, err := s.db.ExecContext(ctx, "UPDATE `task` SET `status` =?, `description` = ? WHERE id = ?",
		request.Task.Status, request.Task.Description, request.Task.Id)

	if err != nil {
		return nil, fmt.Errorf("update task: %v", err)
	}

	task, err := s.getTaskById(ctx, request.Task.Id)

	return &v1.UpdateTaskResponse{Task: task}, err
}

func (s *todoServiceServer) DeleteTask(ctx context.Context, request *v1.DeleteTaskRequest) (*v1.DeleteTaskResponse, error) {
	_, err := s.db.ExecContext(ctx, "DELETE  FROM `task` WHERE id = ?",
		request.Id)

	if err != nil {
		return &v1.DeleteTaskResponse{Success: false}, fmt.Errorf("delete task: %v", err)
	}

	return &v1.DeleteTaskResponse{Success: true}, nil
}

func (s *todoServiceServer) ListTasksStream(request *v1.ListTaskStreamRequest, stream v1.Todo_ListTasksStreamServer) error {
	ctx := stream.Context()
	totalCount, err := s.countTaskRecord(ctx)

	if err != nil {
		return status.Errorf(codes.Internal, "failed to countUserRecord: %+v", err)
	}

	var concurrency = int(request.Concurrency)
	if concurrency == 0 {
		concurrency = 1
	}

	var offset = int(request.Offset)

	var limit = int(request.Limit)
	if limit == 0 {
		limit = 100
	}

	for {
		if offset >= totalCount {
			break
		}

		eg, egCtx := errgroup.WithContext(ctx)

		for i := 0; i < concurrency; i++ {
			nextOffset := offset + i*limit

			if nextOffset >= totalCount {
				break
			}

			eg.Go(func() error {
				records, err := s.searchTaskRecord(egCtx, limit, nextOffset)
				if err != nil {
					return status.Errorf(codes.Internal, "failed to searchTaskRecord: %+v", err)
				}

				taskResponse := &v1.ListTaskStreamResponse{
					Tasks:  records,
					Total:  uint32(totalCount),
					Limit:  request.Limit,
					Offset: uint32(nextOffset),
				}
				err = stream.Send(taskResponse)
				if err != nil {
					return status.Errorf(codes.Internal, "failed to searchUserRecord: %+v", err)
				}

				return nil
			})
		}
		if err := eg.Wait(); err != nil {
			return status.Errorf(codes.Internal, "failed to Wait: %+v", err)
		}

		offset = offset + int(request.Limit)*int(request.Concurrency)
	}

	return nil
}

func (s *todoServiceServer) ListTasks(ctx context.Context, request *v1.ListTaskRequest) (*v1.ListTaskResponse, error) {
	totalCount, err := s.countTaskRecord(ctx)

	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to countUserRecord: %+v", err)
	}

	var offset = int(request.Offset)
	var limit = int(request.Limit)
	if limit == 0 {
		limit = 100
	}

	records, err := s.searchTaskRecord(ctx, limit, offset)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to searchTaskRecord: %+v", err)
	}

	taskResponse := &v1.ListTaskResponse{
		Tasks:  records,
		Total:  uint32(totalCount),
		Limit:  request.Limit,
		Offset: uint32(offset),
	}

	return taskResponse, nil
}
