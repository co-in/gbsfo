package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"
)

type request struct {
	name         string
	method       string
	url          string
	authToken    func() string
	request      func() map[string]interface{}
	statusCode   int
	saveResponse func(map[string]interface{})
}

func (m *request) serializeRequest() io.Reader {
	if m.request == nil {
		return nil
	}

	jsonData, _ := json.Marshal(m.request())
	//fmt.Printf("\nRequest %s\n", jsonData)

	return bytes.NewBuffer(jsonData)
}

func (m *request) processRequest() error {
	data := m.serializeRequest()
	req, err := http.NewRequest(m.method, "http://"+*HTTPPort+m.url, data)
	if err != nil {
		return fmt.Errorf("(%s) Create request: %v", m.name, err)
	}

	if m.authToken != nil {
		req.Header.Set("Authorization", m.authToken())
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("(%s) Post: %v", m.name, err)
	}

	if m.statusCode != 0 && resp.StatusCode != m.statusCode {
		body, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("(%s) Expect[%d], Recv[%d]: %s", m.name, m.statusCode, resp.StatusCode, body)
	}

	var res map[string]interface{}

	err = json.NewDecoder(resp.Body).Decode(&res)
	if err != nil {
		return fmt.Errorf("(%s) Decode response: %v", m.name, err)
	}

	fmt.Printf("PASS: (%s): %v\n", m.name, res)

	if m.saveResponse != nil {
		m.saveResponse(res)
	}

	return nil
}

var token string
var requests = []request{
	{
		name:   "User Not Found",
		method: "POST",
		url:    "/v1/auth/login",
		request: func() map[string]interface{} {
			return map[string]interface{}{
				"login": "admin",
				"pass":  "qwerty",
			}
		},
		statusCode: 404,
	},
	{
		name:   "User SignUp",
		method: "POST",
		url:    "/v1/auth/sign-up",
		request: func() map[string]interface{} {
			return map[string]interface{}{
				"login": "admin",
				"pass":  "qwerty",
			}
		},
		statusCode: 200,
	},
	{
		name:   "User Login Invalid",
		method: "POST",
		url:    "/v1/auth/login",
		request: func() map[string]interface{} {
			return map[string]interface{}{
				"login": "admin",
				"pass":  "fake",
			}
		},
		statusCode: 404,
	},
	{
		name:   "User Login Ok",
		method: "POST",
		url:    "/v1/auth/login",
		request: func() map[string]interface{} {
			return map[string]interface{}{
				"login": "admin",
				"pass":  "qwerty",
			}
		},
		statusCode: 200,
		saveResponse: func(m map[string]interface{}) {
			token = m["token"].(string)
		},
	},
	{
		name:   "User Already Exist",
		method: "POST",
		url:    "/v1/auth/sign-up",
		request: func() map[string]interface{} {
			return map[string]interface{}{
				"login": "admin",
				"pass":  "qwerty",
			}
		},
		statusCode: 500,
	},
	{
		name:   "Check Token Ok",
		method: "POST",
		url:    "/v1/auth/check",
		request: func() map[string]interface{} {
			return map[string]interface{}{
				"token": token,
			}
		},
		statusCode: 200,
	},
	{
		name:   "Check Token Invalid",
		method: "POST",
		url:    "/v1/auth/check",
		request: func() map[string]interface{} {
			return map[string]interface{}{
				"token": token + "fake",
			}
		},
		statusCode: 500,
	},
	{
		name:   "Create Task Without Token",
		method: "POST",
		url:    "/v1/todo",
		request: func() map[string]interface{} {
			return map[string]interface{}{
				"status":      false,
				"description": "Fake task",
			}
		},
		statusCode: 403,
	},
	{
		name:   "Create Task Invalid Token",
		method: "POST",
		url:    "/v1/todo",
		authToken: func() string {
			return "fake"
		},
		request: func() map[string]interface{} {
			return map[string]interface{}{
				"status":      false,
				"description": "Fake task",
			}
		},
		statusCode: 500,
	},
	{
		name:   "Get Not Found Task",
		method: "GET",
		url:    "/v1/todo/1",
		authToken: func() string {
			return token
		},
		statusCode: 404,
	},
	{
		name:   "Create Task With Token",
		method: "POST",
		url:    "/v1/todo",
		authToken: func() string {
			return token
		},
		request: func() map[string]interface{} {
			return map[string]interface{}{
				"status":      false,
				"description": "Valid task",
			}
		},
		statusCode: 200,
	},
	{
		name:   "Get Task #1",
		method: "GET",
		url:    "/v1/todo/1",
		authToken: func() string {
			return token
		},
		statusCode: 200,
	},
	{
		name:   "Update Task #1",
		method: "PUT",
		url:    "/v1/todo/1",
		authToken: func() string {
			return token
		},
		request: func() map[string]interface{} {
			return map[string]interface{}{
				"status":      true,
				"description": "Valid task " + strconv.Itoa(int(time.Now().Unix())),
			}
		},
		statusCode: 200,
	},
	{
		name:   "Delete Task #1",
		method: "DELETE",
		url:    "/v1/todo/1",
		authToken: func() string {
			return token
		},
		statusCode: 200,
	},
	{
		name:   "Update Not Found Task",
		method: "PUT",
		url:    "/v1/todo/1",
		authToken: func() string {
			return token
		},
		statusCode: 404,
	},
	{
		name:       "Get All Tasks Without Token",
		method:     "GET",
		url:        "/v1/todo",
		statusCode: 403,
	},
	{
		name:   "Get All Tasks",
		method: "GET",
		url:    "/v1/todo",
		authToken: func() string {
			return token
		},
		statusCode: 200,
	},
}

var HTTPPort = flag.String("port", ":8080", "Gateway port")
var client *http.Client

func main() {
	flag.Parse()
	client = new(http.Client)

	for _, r := range requests {
		err := r.processRequest()
		if err != nil {
			fmt.Printf("ERROR: %v\n", err)
		}
	}
}
