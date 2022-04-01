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

	"github.com/GoogleCloudPlatform/opentelemetry-operations-go/propagator"
	"github.com/itchyny/gojq"
	"github.com/slack-go/slack"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/automaxprocs/maxprocs"
	"go.uber.org/zap"

	"github.com/ww24/api-checker/internal/logger"
	"github.com/ww24/api-checker/internal/tracer"
)

const (
	serviceName     = "api-checker"
	shutdownTimeout = 10 * time.Second
)

var (
	version      string // set by ldflags
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

	log.SetFlags(0)
	if err := logger.InitializeLogger(ctx, serviceName, version); err != nil {
		log.Printf("ERROR logger.InitializeLogger: %+v", err)
		return
	}

	cl := logger.DefaultLogger(ctx)
	if _, err := maxprocs.Set(maxprocs.Logger(cl.Sugar().Infof)); err != nil {
		cl.Error("maxprocs.Set", zap.Error(err))
	}

	tp, err := tracer.New(serviceName, version)
	if err != nil {
		cl.Error("failed to initialize tracer", zap.Error(err))
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := tp.Shutdown(ctx); err != nil {
			cl.Error("failed to shutdown tracer", zap.Error(err))
		}
	}()

	server := http.Server{
		Addr: ":8080",
		Handler: otelhttp.NewHandler(http.HandlerFunc(handler), "server",
			otelhttp.WithMessageEvents(otelhttp.ReadEvents, otelhttp.WriteEvents),
			otelhttp.WithPropagators(propagator.New()),
		),
	}
	if port := os.Getenv("PORT"); port != "" {
		server.Addr = ":" + port
	}
	go func() {
		if err := server.ListenAndServe(); err != nil {
			cl.Error("server.ListenAndServe", zap.Error(err))
		}
	}()

	cl.Info("starting server",
		zap.String("version", version),
		zap.String("addr", server.Addr),
	)

	<-ctx.Done()
	stop()
	cl.Info("shutdown (signal received)")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		panic(err)
	}
}

func handler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	labeler, _ := otelhttp.LabelerFromContext(ctx)

	if r.Method != http.MethodPost {
		http.Error(w, "only POST method is supported", http.StatusMethodNotAllowed)
		labeler.Add(attribute.Bool("error", true))
		return
	}

	// validation
	if !strings.HasPrefix(r.Header.Get("content-type"), "application/json") {
		http.Error(w, "invalid content-type", http.StatusBadRequest)
		labeler.Add(attribute.Bool("error", true))
		return
	}

	dec := json.NewDecoder(r.Body)
	defer r.Body.Close()

	request := &RequestPayload{}
	if err := dec.Decode(&request); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		labeler.Add(attribute.Bool("error", true))
		return
	}

	cl := logger.DefaultLogger(ctx)
	query, err := gojq.Parse(request.Query)
	if err != nil {
		cl.Error("gojq.Parse", zap.Error(err))
		http.Error(w, "failed to parse jq query", http.StatusInternalServerError)
		labeler.Add(attribute.Bool("error", true))
		return
	}

	data, err := fetch(ctx, request)
	if err != nil {
		cl.Error("fetch", zap.Error(err))
		http.Error(w, "failed to request", http.StatusInternalServerError)
		labeler.Add(attribute.Bool("error", true))
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
			cl.Error("gojq.Next", zap.Error(err))
			http.Error(w, "failed to run jq query", http.StatusInternalServerError)
			labeler.Add(attribute.Bool("error", true))
			return
		}

		cl.Info("jq Result", zap.Any("result", v))
		queryResult = v
		if res, ok := v.(bool); ok && res {
			result = res
			break
		}
	}

	cl.Info("Result", zap.Any("result", result))
	if result && slackChannel != "" && slackToken != "" {
		payload, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			cl.Error("json.MarshalIndent", zap.Error(err))
			http.Error(w, "failed to marshal json", http.StatusInternalServerError)
			labeler.Add(attribute.Bool("error", true))
			return
		}
		if err := notify(ctx, slackChannel, slackToken, request.NotificationMessage, bytes.NewReader(payload)); err != nil {
			cl.Error("notify", zap.Error(err))
			http.Error(w, "failed to notify", http.StatusInternalServerError)
			labeler.Add(attribute.Bool("error", true))
			return
		}
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "QueryResult: %#v\n", queryResult)
}

func fetch(ctx context.Context, request *RequestPayload) (interface{}, error) {
	cli := &http.Client{
		Timeout:   10 * time.Second,
		Transport: otelhttp.NewTransport(http.DefaultTransport),
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

	cl := logger.DefaultLogger(ctx)
	cl.Info("fetch",
		zap.String("method", req.Method),
		zap.String("url", req.URL.String()),
	)

	resp, err := cli.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	cl.Info("fetch result", zap.String("status", resp.Status))

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
		cl.Info("response", zap.String("data", string(data)))
	}

	return data, nil
}

func notify(ctx context.Context, channelID, token, message string, payload io.Reader) error {
	cli := &http.Client{
		Timeout:   10 * time.Second,
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}
	api := slack.New(token, slack.OptionHTTPClient(cli))

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
