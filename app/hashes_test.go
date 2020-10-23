package app

import (
	"reflect"
	"testing"
)

func TestHashFromString(t *testing.T) {
	type args struct {
		s string
	}
	tests := []struct {
		name string
		val  string
		want Hash
	}{
		{
			name: "random valid value",
			val:  "6435b2b5-26df-434c-87ca-58ddab49fcc8",
			want: Hash{ 100, 53 , 178, 181, 38 , 223, 67 , 76 , 135, 202, 88, 221, 171, 73, 252, 200 },
		},
		{
			name: "invalid value",
			val:  "435b2b5-26df-434c-87ca-58ddab49fcc8",
			want: Hash{},
		},
		{
			name: "empty value",
			val:  "",
			want: Hash{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HashFromString(tt.val); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("HashFromString() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHash_MarshalText(t *testing.T) {
	tests := []struct {
		name    string
		h       Hash
		want    []byte
		wantErr bool
	}{
		{
			name:    "valid value",
			h:       Hash{ 100, 53 , 178, 181, 38 , 223, 67 , 76 , 135, 202, 88, 221, 171, 73, 252, 200 },
			want:    []byte("6435b2b5-26df-434c-87ca-58ddab49fcc8"),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.h.MarshalText()
			if (err != nil) != tt.wantErr {
				t.Errorf("MarshalText() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("MarshalText() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHash_String(t *testing.T) {
	tests := []struct {
		name string
		h    Hash
		want string
	}{
		{
			name:    "valid value",
			h:       Hash{ 100, 53 , 178, 181, 38 , 223, 67 , 76 , 135, 202, 88, 221, 171, 73, 252, 200 },
			want:    "6435b2b5-26df-434c-87ca-58ddab49fcc8",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.h.String(); got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHash_Valid(t *testing.T) {
	tests := []struct {
		name string
		h    Hash
		want bool
	}{
		{
			name:    "valid value",
			h:       Hash{ 100, 53 , 178, 181, 38 , 223, 67 , 76 , 135, 202, 88, 221, 171, 73, 252, 200 },
			want:    true,
		},
		{
			name:    "invalid value",
			h:       Hash{},
			want:    false,
		},
		{
			name:    "valid value, but mostly nils",
			h:       Hash{100},
			want:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.h.Valid(); got != tt.want {
				t.Errorf("Valid(%s) = %v, want %v", tt.h, got, tt.want)
			}
		})
	}
}

func TestHashes_Contains(t *testing.T) {
	type args struct {
		s Hash
	}
	tests := []struct {
		name string
		h    Hashes
		args args
		want bool
	}{
		{
			name:    "value contained",
			h:       Hashes{Hash{ 100, 53 , 178, 181, 38 , 223, 67 , 76 , 135, 202, 88, 221, 171, 73, 252, 200 }},
			args:    args{Hash{ 100, 53 , 178, 181, 38 , 223, 67 , 76 , 135, 202, 88, 221, 171, 73, 252, 200 }},
			want:    true,
		},
		{
			name:    "value not contained",
			h:       Hashes{Hash{ 100, 53 , 178, 181, 38 , 223, 67 , 76 , 135, 202, 88, 221, 171, 73, 252, 200 }},
			args:    args{Hash{ 101, 53 , 178, 181, 38 , 223, 67 , 76 , 135, 202, 88, 221, 171, 73, 252, 200 }},
			want:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.h.Contains(tt.args.s); got != tt.want {
				t.Errorf("Contains() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHashes_String(t *testing.T) {
	tests := []struct {
		name string
		h    Hashes
		want string
	}{
		{
			name:    "one value",
			h:       Hashes{Hash{ 100, 53 , 178, 181, 38 , 223, 67 , 76 , 135, 202, 88, 221, 171, 73, 252, 200 }},
			want:    "6435b2b5-26df-434c-87ca-58ddab49fcc8",
		},
		{
			name:    "two values",
			h:       Hashes{
				Hash{ 100, 53 , 178, 181, 38 , 223, 67 , 76 , 135, 202, 88, 221, 171, 73, 252, 200 },
				Hash{ 101, 53 , 178, 181, 38 , 223, 67 , 76 , 135, 202, 88, 221, 171, 73, 252, 200 },
			},
			want:    "6435b2b5-26df-434c-87ca-58ddab49fcc8, 6535b2b5-26df-434c-87ca-58ddab49fcc8",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.h.String(); got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}