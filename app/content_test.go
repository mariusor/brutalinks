package app

import (
	"github.com/mariusor/go-littr/internal/config"
	"reflect"
	"testing"
)

func Test_replaceTags(t *testing.T) {
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
<a href='https://brutalinks.git/~marius' rel='mention'>marius</a>`,
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
			want: `some <a href='https://brutalinks.git/t/tag' rel='tag'>tag</a>
<a href='https://brutalinks.git/~marius' rel='mention'>marius</a>`,
		},
		{
			name: "tag",
			args: args{
				Item{
					MimeType: "text/markdown",
					Data:     `some #tag-`,
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
			want: `some <a href='https://brutalinks.git/t/tag' rel='tag'>tag</a>-`,
		},
	}
	Instance = new(Application)
	Instance.BaseURL = "https://brutalinks.git"
	Instance.Conf = &config.Configuration{HostName: "brutalinks.git"}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := replaceTags(tt.args.cur.MimeType, tt.args.cur); got != tt.want {
				t.Errorf("replaceTags() =\n%v, want\n%v", got, tt.want)
			}
		})
	}
}

func Test_loadTags(t *testing.T) {
	tests := []struct {
		name         string
		data         string
		wantTags     TagCollection
		wantMentions TagCollection
	}{
		{
			name: "a-tag",
			data: "a #tag",
			wantTags: TagCollection{
				Tag{
					Type: TagTag,
					Name: "tag",
					URL:  "/t/tag",
				},
			},
			wantMentions: TagCollection{},
		},
		{
			name:     "an-at-mention",
			data:     "a @mention",
			wantTags: TagCollection{},
			wantMentions: TagCollection{
				Tag{
					Type: TagMention,
					Name: "mention",
					URL:  "/~mention",
				},
			},
		},
		{
			name:     "an-at-with-instance-mention",
			data:     "a @mention@brutalinks.git",
			wantTags: TagCollection{},
			wantMentions: TagCollection{
				Tag{
					Type: TagMention,
					Name: "mention",
					URL:  "https://brutalinks.git/~mention",
				},
			},
		},
		{
			name:     "a-tilde-mention",
			data:     "another ~mention",
			wantTags: TagCollection{},
			wantMentions: TagCollection{
				Tag{
					Type: TagMention,
					Name: "mention",
					URL:  "/~mention",
				},
			},
		},
		{
			name:     "a-tilde-with-instance-mention",
			data:     "another ~mention@brutalinks.git",
			wantTags: TagCollection{},
			wantMentions: TagCollection{
				Tag{
					Type: TagMention,
					Name: "mention",
					URL:  "https://brutalinks.git/~mention",
				},
			},
		},
		{
			name: "a-tag-with-dash",
			data: "a #dashed-tag",
			wantTags: TagCollection{
				Tag{
					Type: TagTag,
					Name: "dashed-tag",
					URL:  "/t/dashed-tag",
				},
			},
			wantMentions: TagCollection{},
		},
		{
			name: "a-tag-with-underscore",
			data: "a #underscored_tag",
			wantTags: TagCollection{
				Tag{
					Type: TagTag,
					Name: "underscored_tag",
					URL:  "/t/underscored_tag",
				},
			},
			wantMentions: TagCollection{},
		},
		{
			name: "an-UPPERCASE-tag",
			data: "a #UPPERTAG",
			wantTags: TagCollection{
				Tag{
					Type: TagTag,
					Name: "UPPERTAG",
					URL:  "/t/UPPERTAG",
				},
			},
			wantMentions: TagCollection{},
		},
		{
			name: "multiple-times-same-tag",
			data: "a #tag is just a #tag",
			wantTags: TagCollection{
				Tag{
					Type: TagTag,
					Name: "tag",
					URL:  "/t/tag",
				},
			},
			wantMentions: TagCollection{},
		},
		{
			name: "more-than-six-tag",
			data: "a #tag is just a #tag, and #sometimes it's #not. #hello #june",
			wantTags: TagCollection{
				Tag{
					Type: TagTag,
					Name: "tag",
					URL:  "/t/tag",
				},
				Tag{
					Type: TagTag,
					Name: "sometimes",
					URL:  "/t/sometimes",
				},
				Tag{
					Type: TagTag,
					Name: "not",
					URL:  "/t/not",
				},
				Tag{
					Type: TagTag,
					Name: "hello",
					URL:  "/t/hello",
				},
				Tag{
					Type: TagTag,
					Name: "june",
					URL:  "/t/june",
				},
			},
			wantMentions: TagCollection{},
		},
		{
			name: "release-notes-202006",
			data: `
In this release I added the capability for users to block other users. 
It is not fully up [to spec](https://www.w3.org/TR/activitypub/#block-activity-outbox), as I'm not entirely
sure how to "prevent the blocked user from interacting with any object posted by the actor". 

Most of the time, however I spent working on the go-ap packages, because this feature served
as the start point for a large detour in the storage layer of #fedbox, the #activitypub service that
constitutes the backend of littr.me, which resulted in the capability of storing "private" activities in actors'
outboxes.

This is a very important step forward in removing the need for a custom "activities" collection for #fedbox.

#updates #june #new-release`,
			wantTags: TagCollection{
				Tag{
					Type: TagTag,
					Name: "fedbox",
					URL:  "/t/fedbox",
				},
				Tag{
					Type: TagTag,
					Name: "activitypub",
					URL:  "/t/activitypub",
				},
				Tag{
					Type: TagTag,
					Name: "updates",
					URL:  "/t/updates",
				},
				Tag{
					Type: TagTag,
					Name: "june",
					URL:  "/t/june",
				},
				Tag{
					Type: TagTag,
					Name: "new-release",
					URL:  "/t/new-release",
				},
			},
			wantMentions: TagCollection{},
		},
	}
	// TODO(marius): stop relying on global state
	Instance.BaseURL = ""
	Instance.Conf = nil
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tags, mentions := loadTags(tt.data)
			if !reflect.DeepEqual(tags, tt.wantTags) {
				t.Errorf("loadTags() got tags = %v, want %v", tags, tt.wantTags)
			}
			if !reflect.DeepEqual(mentions, tt.wantMentions) {
				t.Errorf("loadTags() got mentions = %v, want %v", mentions, tt.wantMentions)
			}
		})
	}
}
