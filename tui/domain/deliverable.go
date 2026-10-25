package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	axlr "github.com/underpass-ai/AXLR/domain"
)

// DeliverToolName is axlr_deliver, the host tool a model hands a
// deliverable over with; the result reads its artifacts from its calls.
const DeliverToolName axlr.ToolName = "axlr_deliver"

const (
	// DeliverablesDirectory is where inline deliverables are written in the
	// workspace, one folder per session: .axlr/deliverables/<session>/<name>.
	DeliverablesDirectory = ".axlr/deliverables"
	// MaxDeliverableInlineBytes bounds inline content, which the model sends
	// in its arguments; MaxDeliverableFileBytes bounds a delivered file.
	MaxDeliverableInlineBytes = 256 << 10
	MaxDeliverableFileBytes   = 32 << 20
	maxDeliverableNameBytes   = 128
	maxDeliverableDescription = 300
	maxMediaTypeBytes         = 127
)

var (
	deliverableName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	// A media type's type and subtype as RFC 6838 restricts them, in lower
	// case and without parameters.
	mediaType = regexp.MustCompile(`^[a-z0-9][a-z0-9!#$&^_.+-]{0,62}/[a-z0-9][a-z0-9!#$&^_.+-]{0,62}$`)
	// Device names Windows refuses as a file name, with any extension.
	reservedDeviceNames = map[string]bool{"con": true, "prn": true, "aux": true, "nul": true, "com1": true, "com2": true, "com3": true, "com4": true, "com5": true, "com6": true, "com7": true, "com8": true, "com9": true, "lpt1": true, "lpt2": true, "lpt3": true, "lpt4": true, "lpt5": true, "lpt6": true, "lpt7": true, "lpt8": true, "lpt9": true}
)

// Deliverable is one axlr_deliver call's arguments, validated: a workspace
// file to deliver as it is (Path), or inline content to write under the
// deliverables folder.
type Deliverable struct {
	Name        string
	MediaType   string
	Description string
	// Path is the workspace file, slash-separated and clean; empty for
	// inline content.
	Path    string
	Content string
}

// Inline reports a deliverable whose content the call carries.
func (d Deliverable) Inline() bool { return d.Path == "" }

// Target is where the deliverable is in the workspace: its path, or the
// file inline content is written to for session.
func (d Deliverable) Target(session SessionID) string {
	if !d.Inline() {
		return d.Path
	}
	return DeliverablesDirectory + "/" + string(session) + "/" + d.Name
}

// ParseDeliverable reads and validates axlr_deliver's arguments: name,
// media_type, description and exactly one of path and content; a why is
// accepted and ignored here.
func ParseDeliverable(arguments axlr.JSONValue) (Deliverable, error) {
	var args struct {
		Name        string  `json:"name"`
		MediaType   string  `json:"media_type"`
		Description string  `json:"description"`
		Path        *string `json:"path"`
		Content     *string `json:"content"`
		Why         string  `json:"why"`
	}
	decoder := json.NewDecoder(bytes.NewReader(arguments.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&args); err != nil {
		return Deliverable{}, fmt.Errorf("axlr_deliver takes name, media_type, description and path or content (%s)", strings.TrimPrefix(err.Error(), "json: "))
	}
	if err := validDeliverableName(args.Name); err != nil {
		return Deliverable{}, err
	}
	if len(args.MediaType) > maxMediaTypeBytes || !mediaType.MatchString(args.MediaType) {
		return Deliverable{}, errors.New("media_type must be a lower-case type/subtype without parameters, such as text/markdown or application/json")
	}
	description := strings.TrimSpace(args.Description)
	if description == "" || strings.ContainsAny(description, "\r\n") || utf8.RuneCountInString(description) > maxDeliverableDescription {
		return Deliverable{}, fmt.Errorf("description must be one line of at most %d characters", maxDeliverableDescription)
	}
	d := Deliverable{Name: args.Name, MediaType: args.MediaType, Description: description}
	switch {
	case (args.Path == nil) == (args.Content == nil):
		return Deliverable{}, errors.New("axlr_deliver takes exactly one of path (a workspace file to deliver as it is) and content (text to write as the deliverable)")
	case args.Path != nil:
		relative, err := axlr.NewRelativePath(*args.Path)
		if err != nil {
			return Deliverable{}, fmt.Errorf("path: %w", err)
		}
		d.Path = path.Clean(filepath.ToSlash(string(relative)))
	default:
		if *args.Content == "" {
			return Deliverable{}, errors.New("content is empty; deliver a file by path instead")
		}
		if len(*args.Content) > MaxDeliverableInlineBytes {
			return Deliverable{}, fmt.Errorf("content exceeds %d bytes; write the file and deliver it by path", MaxDeliverableInlineBytes)
		}
		if !utf8.ValidString(*args.Content) {
			return Deliverable{}, errors.New("content must be UTF-8 text")
		}
		d.Content = *args.Content
	}
	return d, nil
}

func validDeliverableName(name string) error {
	if name == "" || len(name) > maxDeliverableNameBytes || !deliverableName.MatchString(name) || strings.Contains(name, "..") || strings.HasSuffix(name, ".") {
		return fmt.Errorf("name must be a file name of letters, digits, '.', '_' and '-', at most %d characters, starting with a letter or a digit, without '..' and not ending in '.'", maxDeliverableNameBytes)
	}
	stem, _, _ := strings.Cut(strings.ToLower(name), ".")
	if reservedDeviceNames[stem] {
		return fmt.Errorf("name %s is a device name on Windows; choose another", name)
	}
	return nil
}

// DeliversInline reports an axlr_deliver call that carries content, which
// writes a file and is judged as a write; a call with a path only reads.
// It reads arguments leniently: a call that would be refused still counts
// as a write when it names content.
func DeliversInline(arguments axlr.JSONValue) bool {
	var args struct {
		Content *json.RawMessage `json:"content"`
	}
	return json.Unmarshal(arguments.Bytes(), &args) == nil && args.Content != nil
}

// deliveredName is the name an axlr_deliver call gives, "" when
// its arguments do not say.
func deliveredName(arguments axlr.JSONValue) string {
	var args struct {
		Name string `json:"name"`
	}
	_ = json.Unmarshal(arguments.Bytes(), &args)
	return args.Name
}
