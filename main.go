package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/itchyny/gojq"
	"github.com/slack-go/slack"
)

var (
	jst          = time.FixedZone("Asia/Tokyo", 9*60*60)
	slackChannel = os.Getenv("SLACK_CHANNEL")
	slackToken   = os.Getenv("SLACK_TOKEN")
)

type RequestPayload struct {
	URL                 string `json:"url"`
	Method              string `json:"method"`
	ContentType         string `json:"content_type,omitempty"`
	Body                string `json:"body,omitempty"` // base64 encoded
	Query               string `json:"query"`          // jq query and result must be boolean
	NotificationMessage string `json:"notification_message"`
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// setup logger
	log.SetFlags(0)
	log.SetOutput(os.Stdout)

	server := http.Server{
		Addr:    ":8080",
		Handler: http.HandlerFunc(handler),
	}
	if port := os.Getenv("PORT"); port != "" {
		server.Addr = ":" + port
	}
	go func() {
		if err := server.ListenAndServe(); err != nil {
			log.Printf("ERROR server.ListenAndServe: %s", err)
		}
	}()

	log.Println("starting server at " + server.Addr)

	<-ctx.Done()
	stop()
	log.Println("shutdown (signal received)")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		panic(err)
	}
}

func handler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if r.Method != http.MethodPost {
		http.Error(w, "only POST method is supported", http.StatusMethodNotAllowed)
		return
	}

	// validation
	if !strings.HasPrefix(r.Header.Get("content-type"), "application/json") {
		http.Error(w, "invalid content-type", http.StatusBadRequest)
		return
	}

	dec := json.NewDecoder(r.Body)
	defer r.Body.Close()

	request := &RequestPayload{}
	if err := dec.Decode(&request); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	query, err := gojq.Parse(request.Query)
	if err != nil {
		log.Printf("ERROR gojq.Parse: %s", err)
		http.Error(w, "failed to parse jq query", http.StatusInternalServerError)
		return
	}

	data, err := fetch(ctx, request)
	if err != nil {
		log.Printf("ERROR fetch: %s", err)
		http.Error(w, "failed to request", http.StatusInternalServerError)
		return
	}

	var queryResult interface{}
	var result bool
	iter := query.RunWithContext(ctx, data)
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, ok := v.(error); ok {
			log.Printf("ERROR gojq.Next: %s", err)
			http.Error(w, "failed to run jq query", http.StatusInternalServerError)
			return
		}

		log.Printf("jq Result: %#v", v)
		queryResult = v
		if res, ok := v.(bool); ok && res {
			result = res
			break
		}
	}

	log.Println("Result:", result)
	if result && slackChannel != "" && slackToken != "" {
		payload, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			log.Printf("ERROR json.MarshalIndent: %s", err)
			http.Error(w, "failed to marshal json", http.StatusInternalServerError)
			return
		}
		if err := notify(ctx, slackChannel, slackToken, request.NotificationMessage, bytes.NewReader(payload)); err != nil {
			log.Printf("ERROR notify: %s", err)
			http.Error(w, "failed to notify", http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "QueryResult: %#v\n", queryResult)
}

func fetch(ctx context.Context, request *RequestPayload) (interface{}, error) {
	cli := http.Client{
		Timeout: 10 * time.Second,
	}

	method := request.Method
	switch method {
	case http.MethodHead, http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete:
	case "":
		method = http.MethodGet
	default:
		return nil, fmt.Errorf("invalid method: %s", method)
	}

	req, err := http.NewRequestWithContext(ctx, method, request.URL, http.NoBody)
	if err != nil {
		return nil, err
	}

	if request.Body != "" {
		body, err := base64.StdEncoding.DecodeString(request.Body)
		if err != nil {
			return nil, err
		}

		req.Body = io.NopCloser(bytes.NewReader(body))
		if request.ContentType != "" {
			req.Header.Set("content-type", request.ContentType)
		}
	}

	log.Printf("%s %s", req.Method, req.URL.String())

	resp, err := cli.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	log.Println("Status:", resp.Status)

	var data interface{}
	contentType := resp.Header.Get("content-type")
	switch {
	case strings.HasPrefix(contentType, "application/json"):
		dec := json.NewDecoder(resp.Body)
		if err := dec.Decode(&data); err != nil {
			return nil, err
		}

	default:
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		log.Printf("response: %s", string(data))
	}

	return data, nil
}

func notify(ctx context.Context, channelID, token, message string, payload io.Reader) error {
	api := slack.New(token)

	if payload != nil {
		_, err := api.UploadFileContext(ctx, slack.FileUploadParameters{
			Reader:         payload,
			Filetype:       "application/json",
			Filename:       "response.json",
			Title:          "Result " + time.Now().In(jst).Format("(2006-01-02 15:04)"),
			InitialComment: message,
			Channels:       []string{channelID},
		})
		if err != nil {
			return err
		}

		return nil
	}

	msgOpts := []slack.MsgOption{
		slack.MsgOptionText(message, true),
	}
	_, _, err := api.PostMessageContext(ctx, channelID, msgOpts...)
	if err != nil {
		return err
	}

	return nil
}
