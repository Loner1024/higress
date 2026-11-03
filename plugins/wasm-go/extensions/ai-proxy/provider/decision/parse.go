package decision

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

const (
	maxQuestions   = 128
	maxChoices     = 255
	maxScoreLevels = 10
)

// SystemOneRequestModel validates a native TypeSafe POST /v1/systemone body
// and returns its model. Everything else stays byte-preserving, so unknown
// fields and future question types reach TypeSafe untouched.
func SystemOneRequestModel(body []byte) (string, error) {
	root, err := decodeObject(body)
	if err != nil {
		return "", fmt.Errorf("systemone request must be a JSON object: %w", err)
	}
	return requiredString(root["model"], "model")
}

// ParseOpenAIRequest decodes an OpenAI Decisions POST /v1/decisions request.
func ParseOpenAIRequest(body []byte) (*Request, error) {
	root, err := decodeObject(body)
	if err != nil {
		return nil, fmt.Errorf("decisions request must be a JSON object: %w", err)
	}
	input, ok := root["input"]
	if !ok {
		return nil, errors.New("input is required")
	}
	input = bytes.TrimSpace(input)
	if len(input) == 0 || bytes.Equal(input, []byte("null")) {
		return nil, errors.New("input must be a string or an array of messages")
	}
	request := &Request{}
	switch input[0] {
	case '"':
		request.Input.State = cloneRaw(input)
	case '[':
		request.Input.Messages = cloneRaw(input)
		request.Input.HasImage = hasInputImage(input)
	default:
		return nil, errors.New("input must be a string or an array of messages")
	}
	request.HasAudio = rawHasEntries(root["audio"])
	request.HasVideo = rawHasEntries(root["videos"])
	request.SafetyIdentifier, _ = optionalString(root["safety_identifier"])
	stream, err := optionalBool(root["stream"])
	if err != nil {
		return nil, errors.New("stream must be a boolean")
	}
	request.Stream = stream

	questionsBody, ok := root["questions"]
	if !ok {
		return nil, errors.New("questions is required")
	}
	questions, err := decodeArray(questionsBody)
	if err != nil {
		return nil, fmt.Errorf("questions must be an array: %w", err)
	}
	if len(questions) == 0 {
		return nil, errors.New("at least one question is required")
	}
	if len(questions) > maxQuestions {
		return nil, fmt.Errorf("at most %d questions are allowed", maxQuestions)
	}

	request.Questions = make([]Question, 0, len(questions))
	for i, questionBody := range questions {
		question, err := parseOpenAIQuestion(questionBody)
		if err != nil {
			return nil, fmt.Errorf("question %d: %w", i, err)
		}
		question.Key = strconv.Itoa(i)
		request.Questions = append(request.Questions, question)
	}
	assignNames(request.Questions)

	model, err := requiredString(root["model"], "model")
	if err != nil {
		return nil, err
	}
	request.Model = model
	return request, nil
}

func parseOpenAIQuestion(body json.RawMessage) (Question, error) {
	raw, err := decodeObject(body)
	if err != nil {
		return Question{}, errors.New("must be an object")
	}
	kind, err := requiredString(raw["type"], "type")
	if err != nil {
		return Question{}, err
	}
	name, err := optionalString(raw["name"])
	if err != nil {
		return Question{}, errors.New("name must be a string")
	}
	question := Question{
		Name:         name,
		Instructions: cloneRaw(raw["instructions"]),
	}
	if !rawPresent(question.Instructions) {
		return Question{}, errors.New("instructions is required")
	}
	switch kind {
	case "predicate":
		question.Type = TypePredicate
	case "choice":
		question.Type = TypeChoice
		choicesBody, ok := raw["choices"]
		if !ok {
			return Question{}, errors.New("choice question requires choices")
		}
		choices, err := decodeArray(choicesBody)
		if err != nil {
			return Question{}, fmt.Errorf("choices must be an array: %w", err)
		}
		if len(choices) < 2 {
			return Question{}, errors.New("choice question requires at least 2 choices")
		}
		if len(choices) > maxChoices {
			return Question{}, fmt.Errorf("choice allows at most %d choices", maxChoices)
		}
		seen := make(map[string]bool, len(choices))
		for i, choiceBody := range choices {
			choiceRaw, err := decodeObject(choiceBody)
			if err != nil {
				return Question{}, fmt.Errorf("choice %d must be an object", i)
			}
			var value any
			if err := json.Unmarshal(choiceRaw["value"], &value); err != nil {
				return Question{}, fmt.Errorf("choice %d value must be a string or boolean", i)
			}
			key, err := choiceKey(value)
			if err != nil {
				return Question{}, fmt.Errorf("choice %d: %w", i, err)
			}
			if seen[key] {
				return Question{}, fmt.Errorf("choice value %q is used more than once", key)
			}
			seen[key] = true
			question.Choices = append(question.Choices, Choice{
				Value:       value,
				Key:         key,
				Description: cloneRaw(choiceRaw["description"]),
			})
		}
	case "score":
		question.Type = TypeScore
		levelsBody, ok := raw["levels"]
		if !ok {
			return Question{}, errors.New("score question requires levels")
		}
		levels, err := decodeArray(levelsBody)
		if err != nil {
			return Question{}, fmt.Errorf("levels must be an array: %w", err)
		}
		if len(levels) < 2 {
			return Question{}, errors.New("score question requires at least 2 levels")
		}
		if len(levels) > maxScoreLevels {
			return Question{}, fmt.Errorf("score allows at most %d levels", maxScoreLevels)
		}
		for i, levelBody := range levels {
			levelRaw, err := decodeObject(levelBody)
			if err != nil {
				return Question{}, fmt.Errorf("level %d must be an object", i)
			}
			label := cloneRaw(levelRaw["label"])
			if _, err := optionalString(label); err != nil || !rawPresent(label) {
				return Question{}, fmt.Errorf("level %d label must be a string", i)
			}
			question.Levels = append(question.Levels, Level{
				Label:       label,
				Description: cloneRaw(levelRaw["description"]),
			})
		}
	case "box":
		question.Type = TypeBox
	default:
		return Question{}, fmt.Errorf("unsupported question type %q", kind)
	}
	return question, nil
}

// ValidateForSystemOne rejects inputs TypeSafe cannot accept. Images are
// rejected until the asset/image path is implemented; audio and video are not
// part of the System One protocol. Box questions need an image, and streaming
// has no System One equivalent.
func (r *Request) ValidateForSystemOne() error {
	if r.Input.HasImage {
		return errors.New("input_image is not supported by the System One route yet")
	}
	if r.HasAudio {
		return errors.New("audio input is not supported by the System One route")
	}
	if r.HasVideo {
		return errors.New("video input is not supported by the System One route")
	}
	if r.Stream {
		return errors.New("stream is not supported on /v1/decisions")
	}
	for _, question := range r.Questions {
		if question.Type == TypeBox {
			return errors.New("box questions are not supported on /v1/decisions because image input is not supported; use /v1/systemone")
		}
	}
	return nil
}

func assignNames(questions []Question) {
	seen := make(map[string]bool, len(questions))
	allNamed := true
	for _, question := range questions {
		if question.Name == "" {
			allNamed = false
			break
		}
		if seen[question.Name] {
			allNamed = false
			break
		}
		seen[question.Name] = true
	}
	if allNamed {
		for i := range questions {
			questions[i].Key = questions[i].Name
		}
	}
}

func hasInputImage(input json.RawMessage) bool {
	var messages []struct {
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(input, &messages); err != nil {
		return false
	}
	for _, message := range messages {
		var parts []struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(message.Content, &parts); err != nil {
			continue
		}
		for _, part := range parts {
			if part.Type == "input_image" {
				return true
			}
		}
	}
	return false
}

func choiceKey(value any) (string, error) {
	switch typed := value.(type) {
	case string:
		return typed, nil
	case bool:
		if typed {
			return "true", nil
		}
		return "false", nil
	default:
		return "", errors.New("value must be a string or boolean")
	}
}

func requiredString(raw json.RawMessage, field string) (string, error) {
	value, err := optionalString(raw)
	if err != nil {
		return "", fmt.Errorf("%s must be a string", field)
	}
	if value == "" {
		return "", fmt.Errorf("%s is required", field)
	}
	return value, nil
}

func optionalString(raw json.RawMessage) (string, error) {
	if !rawPresent(raw) {
		return "", nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", err
	}
	return value, nil
}

func optionalBool(raw json.RawMessage) (bool, error) {
	if !rawPresent(raw) {
		return false, nil
	}
	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		return false, err
	}
	return value, nil
}

func decodeObject(raw json.RawMessage) (map[string]json.RawMessage, error) {
	var value map[string]json.RawMessage
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	if value == nil {
		return nil, errors.New("expected object")
	}
	return value, nil
}

func decodeArray(raw json.RawMessage) ([]json.RawMessage, error) {
	var value []json.RawMessage
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	if value == nil {
		return nil, errors.New("expected array")
	}
	return value, nil
}

func cloneRaw(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}

func rawPresent(raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	return len(raw) > 0 && !bytes.Equal(raw, []byte("null"))
}

func rawHasEntries(raw json.RawMessage) bool {
	if !rawPresent(raw) {
		return false
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err == nil {
		return len(items) > 0
	}
	return true
}

func rawText(raw json.RawMessage) string {
	if !rawPresent(raw) {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err == nil {
		return value
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err == nil {
		return compact.String()
	}
	return string(raw)
}
