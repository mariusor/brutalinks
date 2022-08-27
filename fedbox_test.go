package go_littr

import (
	"net/url"
	"testing"
)

func Test_RawFilterQuery(t *testing.T) {
	{
		var func1 = func() url.Values {
			return nil
		}
		mustBeEmpty := rawFilterQuery(func1)
		testVal := ""
		if mustBeEmpty != testVal {
			t.Errorf("Value must be %q, received %q", testVal, mustBeEmpty)
		}
	}
	{
		var func1 = func() url.Values {
			return url.Values{
				"iri": {"ana", "are", "mere"},
			}
		}
		testVal := "?iri=ana&iri=are&iri=mere"
		anaAreMere := rawFilterQuery(func1)
		if anaAreMere != testVal {
			t.Errorf("Value must be %q string, received %q", testVal, anaAreMere)
		}
	}
	{
		var func1 = func() url.Values {
			return url.Values{
				"iri": {"ana", "are", "mere"},
			}
		}
		var func2 = func() url.Values {
			return url.Values{
				"iri":  {"foo", "bar"},
				"type": {"typ"},
			}
		}
		testVal := "?iri=ana&iri=are&iri=mere&iri=foo&iri=bar&type=typ"
		anaAreMere := rawFilterQuery(func1, func2)
		if anaAreMere != testVal {
			t.Errorf("Value must be %q, received %q", testVal, anaAreMere)
		}
	}
}
