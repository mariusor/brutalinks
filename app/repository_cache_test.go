package app

import (
	pub "github.com/go-ap/activitypub"
	"testing"
)

func Test_cacheAddGet(t *testing.T) {
	type args struct {
		k pub.IRI
		v pub.Item
	}
	tests := []struct {
		name string
		args args
	}{
		{
			"test",
			args{
				pub.IRI("test"),
				new(pub.Object),
			},
		},
	}

	c := cache{
		enabled: true,
		m:       make(map[pub.IRI]pub.Item),
	}

	if len(c.m) != 0 {
		t.Errorf("invalid initalization for cache map, len = %d, expected %d", len(c.m), 0)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c.add(tt.args.k, tt.args.v)
			v, ok := c.m[tt.args.k]
			if !ok {
				t.Errorf("Could not retrieve key %s", tt.args.k)
			}
			if v != tt.args.v {
				t.Errorf("Value retrieved is different: %v, expected %v", v, tt.args.v)
			}

			vv, vok := c.get(tt.args.k)
			if !vok {
				t.Errorf("Getter could not retrieve key %s", tt.args.k)
			}
			if vv != tt.args.v {
				t.Errorf("Value the getter retrieved is different: %v, expected %v", vv, tt.args.v)
			}
		})
	}
}
