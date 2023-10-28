package brutalinks

import (
	"testing"

	vocab "github.com/go-ap/activitypub"
	"github.com/go-ap/cache"
)

func Test_cacheAddGet(t *testing.T) {
	type args struct {
		k vocab.IRI
		v vocab.Item
	}
	tests := []struct {
		name string
		args args
	}{
		{
			"test",
			args{
				vocab.IRI("test"),
				new(vocab.Object),
			},
		},
	}

	c := cache.New(true)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c.Store(tt.args.k, tt.args.v)
			v := c.Load(tt.args.k)
			if v != tt.args.v {
				t.Errorf("Value retrieved is different: %v, expected %v", v, tt.args.v)
			}

			vv := c.Load(tt.args.k)
			if vv != tt.args.v {
				t.Errorf("Value the getter retrieved is different: %v, expected %v", vv, tt.args.v)
			}
		})
	}
}
