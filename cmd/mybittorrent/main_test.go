package main

import (
	"reflect"
	"testing"
)

func Test_decodeBencode(t *testing.T) {
	tests := []struct {
		name           string
		bencodedString string
		want           interface{}
		wantErr        bool
	}{
		{bencodedString: "5:hello", want: "hello"},
		{bencodedString: "10:hello12345", want: "hello12345"},
		{bencodedString: "i52e", want: 52},
		{bencodedString: "i-52e", want: -52},
		{bencodedString: "l5:helloi52ee", want: []interface{}{"hello", 52}},
		{bencodedString: "d3:foo3:bar5:helloi52ee", want: map[interface{}]interface{}{"hello": 52, "foo": "bar"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, err := decodeBencode(tt.bencodedString)
			if (err != nil) != tt.wantErr {
				t.Errorf("decodeBencode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("decodeBencode() got = %v, want %v", got, tt.want)
			}
		})
	}
}
