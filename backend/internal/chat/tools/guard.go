package tools

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"strings"
	"time"

	"urara-vision/backend/internal/chat/apiclient"
)

// idArguments carry a table or source ID, in the order a message names them.
var idArguments = []string{"table_id", "ids", "from_table", "to_table"}

func notFoundMessage(args json.RawMessage) string {
	var obj map[string]any
	_ = json.Unmarshal(args, &obj)
	var named []string
	for _, key := range idArguments {
		switch v := obj[key].(type) {
		case string:
			if v != "" {
				named = append(named, "'"+v+"'")
			}
		case []any:
			for _, id := range v {
				if s, ok := id.(string); ok {
					named = append(named, "'"+s+"'")
				}
			}
		}
	}
	subject := "That was not found"
	if len(named) > 0 {
		subject = "No table with id " + strings.Join(named, ", ")
	}
	return subject + " in this model. Call search_model to find the right ID."
}

// Guarded runs a spec, turning recoverable failures into text for the model.
// Forbidden, a finished turn and anything unexpected come back as errors.
func Guarded(spec Spec, log *slog.Logger) func(ctx context.Context, args json.RawMessage) (any, error) {
	return func(ctx context.Context, args json.RawMessage) (result any, err error) {
		started := time.Now()
		var advice string
		defer func() {
			var logged any = string(args)
			if json.Valid(args) {
				logged = args
			}
			var errText any
			if advice != "" {
				errText = advice
			}
			log.Debug("tool call", "tool", spec.Name, "tool_args", logged,
				"duration_ms", math.Round(float64(time.Since(started).Microseconds())/10)/100,
				"returned_error", advice != "", "error", errText)
		}()

		if verr := Validate(spec.Name, args); verr != nil {
			advice = "Invalid arguments for " + spec.Name + ": " + verr.Error() + ". Fix them and call the tool again."
			return advice, nil
		}
		result, err = spec.Run(ctx, args)
		if err == nil {
			return result, nil
		}

		var backendErr *apiclient.Error
		switch {
		// Forbidden ends the turn with a 403; the model cannot work around it.
		case errors.Is(err, apiclient.ErrForbidden), ctx.Err() != nil:
			return nil, err
		case errors.Is(err, apiclient.ErrNotFound):
			advice = notFoundMessage(args)
		case errors.Is(err, context.DeadlineExceeded):
			advice = "That lookup timed out. Try a narrower query."
		case errors.As(err, &backendErr):
			advice = "The model store could not answer that: " + backendErr.Message + "."
		default:
			return nil, err
		}
		return advice, nil
	}
}
