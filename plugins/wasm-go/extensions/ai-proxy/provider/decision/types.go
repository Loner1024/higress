package decision

import "encoding/json"

// QuestionType is the normalized question kind shared by the System One and
// OpenAI Decisions wire formats.
type QuestionType string

const (
	TypePredicate QuestionType = "predicate"
	TypeChoice    QuestionType = "choice"
	TypeScore     QuestionType = "score"
	TypeBox       QuestionType = "box"
	TypeRefusal   QuestionType = "refusal"
)

// Request is the provider-neutral decision request. A request either carries a
// System One state or an OpenAI decisions input array, never both.
type Request struct {
	Model            string
	Input            Input
	Questions        []Question
	SafetyIdentifier string
	Extras           map[string]json.RawMessage

	HasAudio bool
	HasVideo bool
}

// Input keeps the wire representation it came from. State is set for a System
// One state (string, object, or array); Messages is set for an OpenAI decisions
// input array.
type Input struct {
	State    json.RawMessage
	Messages json.RawMessage
	HasImage bool
}

// Question is one normalized question. Key is the identity used on the System
// One wire: the caller's question name when it is usable, otherwise its index.
type Question struct {
	Type         QuestionType
	Name         string
	Key          string
	Instructions json.RawMessage
	Criteria     json.RawMessage
	Choices      []Choice
	Levels       []Level
}

// Choice is one option of a choice question. Value keeps the OpenAI type
// (string or bool); Key is the System One criteria key.
type Choice struct {
	Value       any
	Key         string
	Description json.RawMessage
}

// Level is one level of a score question. Label is a string on the OpenAI wire
// and may be any JSON value on the System One wire.
type Level struct {
	Label       json.RawMessage
	Description json.RawMessage
}

// Response is the normalized decision response. Answers are keyed by the
// System One question key used on the wire.
type Response struct {
	Model   string
	Answers map[string]Answer
	Usage   *Usage
}

// Answer is one normalized answer. The field set depends on Type.
type Answer struct {
	Type          QuestionType
	Noul          *float64
	Choice        *string
	Score         *float64
	Probabilities map[string]float64
	Legend        map[string]json.RawMessage
	Confidence    *float64
	Box           *Box
}

// Box is a box answer in fractions of the image dimensions.
type Box struct {
	Top    float64 `json:"top"`
	Bottom float64 `json:"bottom"`
	Left   float64 `json:"left"`
	Right  float64 `json:"right"`
}

// Usage reports token consumption.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}
