package decision

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

// SystemOneBody renders the normalized request as a TypeSafe /v1/systemone
// request. OpenAI safety identifiers have no System One equivalent and are not
// carried.
func (r *Request) SystemOneBody() ([]byte, error) {
	if r.Model == "" {
		return nil, errors.New("model is required")
	}
	if len(r.Questions) == 0 {
		return nil, errors.New("at least one question is required")
	}
	state := r.Input.State
	if state == nil {
		state = r.Input.Messages
	}
	if state == nil {
		state = json.RawMessage("null")
	}

	questions := make(map[string]any, len(r.Questions))
	for _, question := range r.Questions {
		key := question.Key
		if key == "" {
			key = question.Name
		}
		if key == "" {
			return nil, errors.New("question key is required")
		}
		if _, exists := questions[key]; exists {
			return nil, fmt.Errorf("question key %q is used more than once", key)
		}
		encoded, err := systemOneQuestion(question)
		if err != nil {
			return nil, err
		}
		questions[key] = encoded
	}

	root := map[string]any{
		"model":     r.Model,
		"state":     state,
		"questions": questions,
	}
	for key, value := range r.Extras {
		if _, exists := root[key]; !exists {
			root[key] = value
		}
	}
	return json.Marshal(root)
}

func systemOneQuestion(question Question) (map[string]any, error) {
	encoded := map[string]any{}
	switch question.Type {
	case TypePredicate:
		encoded["type"] = "noul"
		if rawPresent(question.Instructions) {
			encoded["instructions"] = question.Instructions
		}
		if rawPresent(question.Criteria) {
			encoded["criteria"] = question.Criteria
		}
	case TypeChoice:
		if len(question.Choices) == 0 {
			return nil, errors.New("choice question requires choices")
		}
		criteria := make(map[string]any, len(question.Choices))
		for _, choice := range question.Choices {
			criteria[choice.Key] = nullableRaw(choice.Description)
		}
		encoded["type"] = "choice"
		if rawPresent(question.Instructions) {
			encoded["instructions"] = question.Instructions
		}
		encoded["criteria"] = criteria
	case TypeScore:
		if len(question.Levels) == 0 {
			return nil, errors.New("score question requires levels")
		}
		criteria := make([]any, 0, len(question.Levels))
		for _, level := range question.Levels {
			if rawPresent(level.Description) {
				criteria = append(criteria, fmt.Sprintf("%s: %s", rawText(level.Label), rawText(level.Description)))
			} else {
				criteria = append(criteria, nullableRaw(level.Label))
			}
		}
		encoded["type"] = "score"
		if rawPresent(question.Instructions) {
			encoded["instructions"] = question.Instructions
		}
		encoded["criteria"] = criteria
	case TypeBox:
		encoded["type"] = "box"
		if rawPresent(question.Instructions) {
			encoded["instructions"] = question.Instructions
		}
	default:
		return nil, fmt.Errorf("unsupported question type %q", question.Type)
	}
	return encoded, nil
}

// OpenAIResponseBody renders a System One response in the OpenAI Decisions
// shape, matching answers to the request's names and order. A missing or
// mismatched answer becomes a refusal.
func (r *Request) OpenAIResponseBody(response *Response) ([]byte, error) {
	if response == nil {
		return nil, errors.New("response is required")
	}
	answers := make([]any, 0, len(r.Questions))
	for _, question := range r.Questions {
		answers = append(answers, openAIAnswer(question, response.Answers[question.Key]))
	}
	usage := response.Usage
	if usage == nil {
		usage = &Usage{}
	}
	model := response.Model
	if model == "" {
		model = r.Model
	}
	return json.Marshal(map[string]any{
		"model":   model,
		"answers": answers,
		"usage": map[string]any{
			"input_tokens":          usage.InputTokens,
			"input_tokens_details":  map[string]int{"cached_tokens": 0, "cache_write_tokens": 0},
			"output_tokens":         usage.OutputTokens,
			"output_tokens_details": map[string]int{"reasoning_tokens": 0},
			"total_tokens":          usage.InputTokens + usage.OutputTokens,
		},
	})
}

func openAIAnswer(question Question, answer Answer) map[string]any {
	encoded := map[string]any{"type": string(question.Type)}
	if question.Name != "" {
		encoded["name"] = question.Name
	}
	if answer.Type != question.Type {
		encoded["type"] = string(TypeRefusal)
		return encoded
	}
	switch question.Type {
	case TypePredicate:
		if answer.Noul == nil {
			encoded["type"] = string(TypeRefusal)
			return encoded
		}
		encoded["probability"] = *answer.Noul
	case TypeChoice:
		if answer.Choice == nil {
			encoded["type"] = string(TypeRefusal)
			return encoded
		}
		choice := any(*answer.Choice)
		for _, option := range question.Choices {
			if option.Key == *answer.Choice {
				choice = option.Value
				break
			}
		}
		probabilities := make([]any, 0, len(question.Choices))
		for _, option := range question.Choices {
			probabilities = append(probabilities, map[string]any{
				"value":       option.Value,
				"probability": answer.Probabilities[option.Key],
			})
		}
		encoded["choice"] = choice
		encoded["probabilities"] = probabilities
		encoded["confidence"] = numberOrZero(answer.Confidence)
	case TypeScore:
		if answer.Score == nil {
			encoded["type"] = string(TypeRefusal)
			return encoded
		}
		probabilities := make([]any, 0, len(question.Levels))
		for i, level := range question.Levels {
			probabilities = append(probabilities, map[string]any{
				"value":       i,
				"label":       rawText(level.Label),
				"probability": answer.Probabilities[strconv.Itoa(i)],
			})
		}
		encoded["score"] = *answer.Score
		encoded["probabilities"] = probabilities
		encoded["confidence"] = numberOrZero(answer.Confidence)
	case TypeBox:
		if answer.Box == nil {
			encoded["type"] = string(TypeRefusal)
			return encoded
		}
		encoded["box"] = answer.Box
		encoded["confidence"] = numberOrZero(answer.Confidence)
	}
	return encoded
}

// SystemOneResponseBody renders a normalized response in the System One shape.
// It is used when an OpenAI decisions request is served by a System One
// upstream, and is also the inverse of ParseSystemOneResponse.
func (r *Request) SystemOneResponseBody(response *Response) ([]byte, error) {
	if response == nil {
		return nil, errors.New("response is required")
	}
	answers := make(map[string]any, len(r.Questions))
	for _, question := range r.Questions {
		answer, ok := response.Answers[question.Key]
		if !ok || answer.Type != question.Type {
			continue
		}
		encoded, err := systemOneAnswer(question, answer)
		if err != nil {
			return nil, err
		}
		answers[question.Key] = encoded
	}
	usage := response.Usage
	if usage == nil {
		usage = &Usage{}
	}
	model := response.Model
	if model == "" {
		model = r.Model
	}
	return json.Marshal(map[string]any{
		"model":   model,
		"answers": answers,
		"usage":   usage,
	})
}

func systemOneAnswer(question Question, answer Answer) (map[string]any, error) {
	encoded := map[string]any{"type": string(question.Type)}
	switch question.Type {
	case TypePredicate:
		if answer.Noul == nil {
			return nil, fmt.Errorf("answer %q has no noul value", question.Key)
		}
		encoded["type"] = "noul"
		encoded["noul"] = *answer.Noul
	case TypeChoice:
		if answer.Choice == nil {
			return nil, fmt.Errorf("answer %q has no choice value", question.Key)
		}
		encoded["type"] = "choice"
		encoded["choice"] = *answer.Choice
		encoded["probabilities"] = answer.Probabilities
		if answer.Confidence != nil {
			encoded["confidence"] = *answer.Confidence
		}
	case TypeScore:
		if answer.Score == nil {
			return nil, fmt.Errorf("answer %q has no score value", question.Key)
		}
		legend := make(map[string]string, len(question.Levels))
		for i, level := range question.Levels {
			legend[strconv.Itoa(i)] = rawText(level.Label)
		}
		encoded["type"] = "score"
		encoded["score"] = *answer.Score
		encoded["legend"] = legend
		encoded["probabilities"] = answer.Probabilities
		if answer.Confidence != nil {
			encoded["confidence"] = *answer.Confidence
		}
	case TypeBox:
		if answer.Box == nil {
			return nil, fmt.Errorf("answer %q has no box value", question.Key)
		}
		encoded["type"] = "box"
		encoded["box"] = answer.Box
		if answer.Confidence != nil {
			encoded["confidence"] = *answer.Confidence
		}
	}
	return encoded, nil
}

// ParseSystemOneResponse decodes a TypeSafe /v1/systemone response.
func ParseSystemOneResponse(body []byte) (*Response, error) {
	root, err := decodeObject(body)
	if err != nil {
		return nil, fmt.Errorf("systemone response must be a JSON object: %w", err)
	}
	response := &Response{
		Model:   rawText(root["model"]),
		Answers: make(map[string]Answer),
	}
	if usageBody, ok := root["usage"]; ok {
		var usage Usage
		if err := json.Unmarshal(usageBody, &usage); err == nil {
			response.Usage = &usage
		}
	}
	answersBody, ok := root["answers"]
	if !ok {
		return response, nil
	}
	answers, err := decodeObject(answersBody)
	if err != nil {
		return nil, fmt.Errorf("answers must be an object: %w", err)
	}
	for key, body := range answers {
		answer, err := parseSystemOneAnswer(body)
		if err != nil {
			return nil, fmt.Errorf("answer %q: %w", key, err)
		}
		response.Answers[key] = answer
	}
	return response, nil
}

func parseSystemOneAnswer(body json.RawMessage) (Answer, error) {
	var raw struct {
		Type          string                     `json:"type"`
		Noul          *float64                   `json:"noul"`
		Choice        *string                    `json:"choice"`
		Score         *float64                   `json:"score"`
		Probabilities map[string]float64         `json:"probabilities"`
		Legend        map[string]json.RawMessage `json:"legend"`
		Confidence    *float64                   `json:"confidence"`
		Box           *Box                       `json:"box"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return Answer{}, err
	}
	answer := Answer{
		Probabilities: raw.Probabilities,
		Legend:        raw.Legend,
		Confidence:    raw.Confidence,
		Noul:          raw.Noul,
		Choice:        raw.Choice,
		Score:         raw.Score,
		Box:           raw.Box,
	}
	switch raw.Type {
	case "noul":
		answer.Type = TypePredicate
	case "choice":
		answer.Type = TypeChoice
	case "score":
		answer.Type = TypeScore
	case "box":
		answer.Type = TypeBox
	case "refusal":
		answer.Type = TypeRefusal
	default:
		return Answer{}, fmt.Errorf("unsupported answer type %q", raw.Type)
	}
	return answer, nil
}

func nullableRaw(raw json.RawMessage) any {
	if !rawPresent(raw) {
		return nil
	}
	return raw
}

func numberOrZero(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}
