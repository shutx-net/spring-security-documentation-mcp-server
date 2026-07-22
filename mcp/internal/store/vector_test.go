package store

import (
	"reflect"
	"testing"
)

func TestVectorFilterDoc(t *testing.T) {
	tests := []struct {
		name string
		ref  string
		area string
		want map[string]interface{}
	}{
		{
			name: "no fields",
			want: nil,
		},
		{
			name: "ref only",
			ref:  "6.5.x",
			want: map[string]interface{}{"ref": map[string]interface{}{"$eq": "6.5.x"}},
		},
		{
			name: "area only",
			area: "servlet",
			want: map[string]interface{}{"area": map[string]interface{}{"$eq": "servlet"}},
		},
		{
			name: "ref and area use explicit $and",
			ref:  "6.5.x",
			area: "servlet",
			want: map[string]interface{}{"$and": []map[string]interface{}{
				{"ref": map[string]interface{}{"$eq": "6.5.x"}},
				{"area": map[string]interface{}{"$eq": "servlet"}},
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := vectorFilterDoc(tt.ref, tt.area)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("vectorFilterDoc(%q, %q) = %#v, want %#v", tt.ref, tt.area, got, tt.want)
			}
		})
	}
}
