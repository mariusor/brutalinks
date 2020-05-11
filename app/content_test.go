package app

import (
	"testing"
)

func Test_replaceTagsInItem(t *testing.T) {
	type args struct {
		cur Item
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "mention-but-not-in-url",
			args: args{
				Item{
					MimeType: "text/markdown",
					Data: `https://todo.sr.ht/~marius/go-activitypub
~marius`,
					Metadata: &ItemMetadata{
						Mentions: TagCollection{
							Tag{
								Type: TagMention,
								Name: "marius",
								URL:  "https://brutalinks.git/~marius",
							},
						},
					},
				},
			},
			want: `https://todo.sr.ht/~marius/go-activitypub
<a href='https://brutalinks.git/~marius' rel='mention' class='mention'>marius</a>`,
		},
		{
			name: "mention-and-tag",
			args: args{
				Item{
					MimeType: "text/markdown",
					Data: `some #tag
~marius`,
					Metadata: &ItemMetadata{
						Mentions: TagCollection{
							Tag{
								Type: TagMention,
								Name: "marius",
								URL:  "https://brutalinks.git/~marius",
							},
						},
						Tags: TagCollection{
							Tag{
								Type: TagTag,
								Name: "tag",
								URL:  "https://brutalinks.git/t/tag",
							},
						},
					},
				},
			},
			want: `some <a href='https://brutalinks.git/t/tag' rel='tag' class='tag'>tag</a>
<a href='https://brutalinks.git/~marius' rel='mention' class='mention'>marius</a>`,
		},
		{
			name: "tag",
			args: args{
				Item{
					MimeType: "text/markdown",
					Data: `some #tag-`,
					Metadata: &ItemMetadata{
						Tags: TagCollection{
							Tag{
								Type: TagTag,
								Name: "tag",
								URL:  "https://brutalinks.git/t/tag",
							},
						},
					},
				},
			},
			want: `some <a href='https://brutalinks.git/t/tag' rel='tag' class='tag'>tag</a>-`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := replaceTagsInItem(tt.args.cur); got != tt.want {
				t.Errorf("replaceTagsInItem() =\n%v, want\n%v", got, tt.want)
			}
		})
	}
}
