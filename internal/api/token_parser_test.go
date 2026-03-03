package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseConsumerTokens(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    map[string]string
		wantErr string
	}{
		{
			name: "single consumer",
			raw:  "app1:token1",
			want: map[string]string{"app1": "token1"},
		},
		{
			name: "multiple consumers",
			raw:  "app1:token1,myapp:token2",
			want: map[string]string{"app1": "token1", "myapp": "token2"},
		},
		{
			name: "three consumers",
			raw:  "app1:secret1,myapp:secret2,another:secret3",
			want: map[string]string{"app1": "secret1", "myapp": "secret2", "another": "secret3"},
		},
		{
			name: "whitespace trimming",
			raw:  " app1 : token1 , myapp : token2 ",
			want: map[string]string{"app1": "token1", "myapp": "token2"},
		},
		{
			name:    "empty string",
			raw:     "",
			wantErr: "no consumer tokens configured",
		},
		{
			name:    "whitespace only",
			raw:     "   ",
			wantErr: "no consumer tokens configured",
		},
		{
			name:    "missing colon",
			raw:     "app1-token1",
			wantErr: "invalid consumer token entry \"app1-token1\": expected format \"name:token\"",
		},
		{
			name:    "missing colon in second entry",
			raw:     "app1:token1,badentry",
			wantErr: "invalid consumer token entry \"badentry\": expected format \"name:token\"",
		},
		{
			name:    "empty name",
			raw:     ":token1",
			wantErr: "invalid consumer token entry \":token1\": name and token must not be empty",
		},
		{
			name:    "empty token",
			raw:     "app1:",
			wantErr: "invalid consumer token entry \"app1:\": name and token must not be empty",
		},
		{
			name:    "duplicate consumer name",
			raw:     "app1:token1,app1:token2",
			wantErr: "duplicate consumer name \"app1\"",
		},
		{
			name: "token containing colon",
			raw:  "app1:tok:en:1",
			want: map[string]string{"app1": "tok:en:1"},
		},
		{
			name:    "duplicate token value",
			raw:     "app1:sametoken,app2:sametoken",
			wantErr: "duplicate token value shared by consumers \"app1\" and \"app2\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseConsumerTokens(tt.raw)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Equal(t, tt.wantErr, err.Error())
				assert.Nil(t, got)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
