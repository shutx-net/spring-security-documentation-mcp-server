package eval

import (
	"encoding/json"
	"io"
)

// WriteJSONReport encodes report as indented JSON to w.
func WriteJSONReport(w io.Writer, report *RunReport) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}
