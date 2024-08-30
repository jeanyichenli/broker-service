package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/rpc"
	"time"

	"github.com/jeanyichenli/broker-service/event"
	"github.com/jeanyichenli/broker-service/logs"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type ResponsePayload struct {
	Action string      `json:"action"`
	Auth   AuthPayload `json:"auth,omitempty"`
	Log    LogPayload  `json:"log,omitempty"`
	Mail   MailPayload `json:"mail,omitempty"`
}

type AuthPayload struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LogPayload struct {
	Name string `json:"name"`
	Data string `json:"data"`
}

type MailPayload struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Subject string `json:"subject"`
	Message string `json:"message"`
}

func (app *Config) Broker(w http.ResponseWriter, r *http.Request) {
	payload := jsonResponse{
		Error:   false,
		Message: "Hit the broker",
	}

	_ = app.writeJson(w, http.StatusOK, payload)

	// // json marshal
	// out, _ := json.MarshalIndent(payload, "", "\t")

	// // write the response
	// w.Header().Set("Content-Type", "application/json")
	// w.WriteHeader(http.StatusAccepted)
	// w.Write(out)
}

func (app *Config) HandleSubmission(w http.ResponseWriter, r *http.Request) {
	var responsePayload ResponsePayload

	err := app.readJson(w, r, &responsePayload)
	if err != nil {
		app.errorJson(w, err, http.StatusBadRequest)
		return
	}

	switch responsePayload.Action {
	case "auth":
		app.authenticate(w, responsePayload.Auth)
	case "log":
		app.logItemViaRPC(w, responsePayload.Log)
	case "mail":
		app.sendMail(w, responsePayload.Mail)
	default:
		app.errorJson(w, errors.New("unknwon action"), http.StatusBadRequest)
	}
}

// Push messsage directly to auth service
func (app *Config) authenticate(w http.ResponseWriter, a AuthPayload) {
	// create some json we'll send to the auth service
	jsonData, _ := json.MarshalIndent(a, "", "\t") // In production, prefer to use json.Marshal

	// call the service (with HTTP request)
	request, err := http.NewRequest("POST", "http://authentication-service/authenticate", bytes.NewBuffer(jsonData)) // use docker dns
	if err != nil {
		app.errorJson(w, err)
		return
	}
	request.Header.Set("Content-type", "application/json")

	client := http.Client{}
	response, err := client.Do(request)
	if err != nil {
		app.errorJson(w, err)
		return
	}
	defer response.Body.Close()

	// make sure we get back the correct status code
	if response.StatusCode == http.StatusUnauthorized {
		app.errorJson(w, errors.New("invalid credentials"))
		return
	} else if response.StatusCode != http.StatusAccepted {
		app.errorJson(w, errors.New("error calling auth service"))
		return
	}

	// make a variable we'll read response.Body into
	var jsonFromService jsonResponse

	err = json.NewDecoder(response.Body).Decode(&jsonFromService)
	if err != nil {
		app.errorJson(w, err, http.StatusUnauthorized)
		return
	}

	var payload jsonResponse
	payload.Error = false
	payload.Message = "Authenticate!"
	payload.Data = jsonFromService.Data

	app.writeJson(w, http.StatusAccepted, payload)
}

// Push messsage directly to logger service
func (app *Config) logItem(w http.ResponseWriter, entry LogPayload) {
	jsonData, _ := json.MarshalIndent(entry, "", "\t")

	logServiceUrl := "http://logger-service/logs"
	request, err := http.NewRequest("POST", logServiceUrl, bytes.NewBuffer(jsonData))
	if err != nil {
		app.errorJson(w, err)
		return
	}

	request.Header.Set("Content-type", "application/json")

	client := http.Client{}
	response, err := client.Do(request)
	if err != nil {
		app.errorJson(w, err)
		return
	}
	defer request.Body.Close()

	if response.StatusCode != http.StatusAccepted {
		app.errorJson(w, err)
		return
	}

	var payload jsonResponse
	payload.Error = false
	payload.Message = "logged"

	app.writeJson(w, http.StatusAccepted, payload)
}

// Push messsage directly to mailer service
func (app *Config) sendMail(w http.ResponseWriter, msg MailPayload) {
	jsonData, _ := json.MarshalIndent(msg, "", "\t")

	mailServiceUrl := "http://mail-service/send" // inside docker containers
	request, err := http.NewRequest("POST", mailServiceUrl, bytes.NewBuffer(jsonData))
	if err != nil {
		app.errorJson(w, err)
		return
	}

	request.Header.Set("Content-type", "application/json")

	client := &http.Client{}
	response, err := client.Do(request)
	if err != nil {
		app.errorJson(w, err)
		return
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusAccepted {
		app.errorJson(w, errors.New("Error calling mail service."))
		return
	}

	var responsePayload jsonResponse

	responsePayload.Error = false
	responsePayload.Message = "Sent to " + msg.To

	app.writeJson(w, http.StatusAccepted, responsePayload)
}

// write log via RabbitMQ
func (app *Config) logEventViaRabbit(w http.ResponseWriter, l LogPayload) {
	fmt.Println("logEventViaRabbit")
	err := app.pushToQueue(l.Name, l.Data)
	if err != nil {
		app.errorJson(w, err)
		return
	}

	var payload jsonResponse
	payload.Error = false
	payload.Message = "logged via RabbitMQ"

	app.writeJson(w, http.StatusAccepted, payload)
}

func (app *Config) pushToQueue(name, msg string) error {
	fmt.Println("pushToQueue")
	emitter, err := event.NewEventEmitter(app.Rabbit)
	if err != nil {
		return err
	}

	payload := LogPayload{
		Name: name,
		Data: msg,
	}

	jsonData, _ := json.MarshalIndent(&payload, "", "\t")

	err = emitter.Push(string(jsonData), "log.INFO")
	if err != nil {
		fmt.Println("Push error:", err)
		return err
	}

	fmt.Println("Push done")

	return nil
}

type RPCPayload struct {
	Name string
	Data string
}

// write logs via RPC call
func (app *Config) logItemViaRPC(w http.ResponseWriter, l LogPayload) {
	rpcClient, err := rpc.Dial("tcp", "logger-service:5001")
	if err != nil {
		app.errorJson(w, err)
		return
	}

	// rpcPayload := RPCPayload{
	// 	Name: l.Name,
	// 	Data: l.Data,
	// }

	var result string
	err = rpcClient.Call("RPCServer.LogInfo", l, &result)
	if err != nil {
		app.errorJson(w, err)
		return
	}

	payload := jsonResponse{
		Error:   false,
		Message: result,
	}

	app.writeJson(w, http.StatusAccepted, payload)
}

func (app *Config) LogViaGRPC(w http.ResponseWriter, r *http.Request) {
	var requestPayload ResponsePayload

	err := app.readJson(w, r, &requestPayload)
	if err != nil {
		app.errorJson(w, err)
		return
	}

	// grpc connection
	conn, err := grpc.NewClient("logger-service:50001", grpc.WithTransportCredentials(insecure.NewCredentials())) // 課程使用舊版grpc package, 使用grpc.Dial (deprecated)
	if err != nil {
		app.errorJson(w, err)
		return
	}
	defer conn.Close()

	// grpc client
	grpcClient := logs.NewLogServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second) // very fast transport, so timeout can set to one second
	defer cancel()

	_, err = grpcClient.WriteLog(ctx, &logs.LogRequest{
		LogEntry: &logs.Log{
			Name: requestPayload.Log.Name,
			Data: requestPayload.Log.Data,
		},
	})
	if err != nil {
		app.errorJson(w, err)
		return
	}

	// response
	payload := jsonResponse{
		Error:   false,
		Message: "logged via gRPC!",
	}

	app.writeJson(w, http.StatusAccepted, payload)
}
