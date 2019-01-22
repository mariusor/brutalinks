// Package processing offers a specialized wrapper over existing queue libraries.
package processing

import (
	as "github.com/go-ap/activitystreams"
	"github.com/juju/errors"
	"github.com/mariusor/littr.go/app"
	"github.com/mariusor/littr.go/app/log"
)

var Logger log.Logger

type (
	EntityType string
	ActionType string
)

const (
	TypeItem    EntityType = "item"
	TypeAccount EntityType = "account"

	// Example: we want to send an item to an external inbox
	// {"key": "1234568", "type": "item", "action": { "verb": "federate", "target": "https://external.example.com/accounts/jane_doe/inbox"}}
	// Loads content_item.Key = 1234568, composes a activityStreams.Create activity with it
	// PUTs to the Object ID of the external inbox
	// TODO(marius): store this id in the message payload
)

// execute GenerateSSHKey for the "system" user
var _exSSH = Message{
	Priority: PriorityLow,
	Actions: []interface{}{
		SSHKey{
			Type: "id-rsa",
			Hash: app.Hash("system"),
			Seed: 666,
		},
	},
}

// execute ScoreUpdate for the "system" and "anonymous" users
var _exVoteAccount = Message{
	Priority: PriorityLow,
	Actions: []interface{}{
		ScoreUpdate{
			Type: TypeAccount,
			Hash: app.Hash("29fc2269252dd76fa7e4b6d193f51a3f3cd21fdf30e44f34ec138d7e803cf0c3"), // system
		},
		ScoreUpdate{
			Type: TypeAccount,
			Hash: app.Hash("77b7b7215e8d78452dc40da9efbb65fdc918c757844387aa0f88143762495c6b"), // anonymous
		},
	},
}

// execute ScoreUpdate for the content_item with the corresponding hash
var _exVoteItem = Message{
	Priority: PriorityLow,
	Actions: []interface{}{
		ScoreUpdate{
			Type: TypeItem,
			Hash: app.Hash("cb615f8863b197b86a08354911b93c0fc3d365061a83bb6482f8ac67c871d192"), // about littr.me
		},
	},
}

// TODO(marius): I think I can use a single Action for processing both incoming and outgoing AP activities
//   as long as we can base ourselves on the Actor and Audience fields.
//   Actor is local, audience is world: message must be outgoing
//   Actor is local, audience is local: message must be local (targeting api/self/outbox probably)
//   Actor is world, audience is local: message must be incoming
//   Actor is world, audience is world: unknown, ignore probably
// execute Process Incoming ActivityPub payload
var _exProcessIncomingInboxAction = Message{
	Priority: PriorityHigh,
	Actions: []interface{}{
		APProcess{
			Activity: as.Activity{
				Parent: as.Parent{
					Type: as.CreateType,
				},
				Actor: as.IRI("https://external.example.com/accounts/jane_doe"),
				Object: &as.Object{
					ID:        as.ObjectID("https://external.example.com/accounts/jane_doe/outbox/special-note-identifier-123"),
					Type:      as.NoteType,
					InReplyTo: as.IRI("https://littr.git/api/actors/system/outbox/7ca154ff"),
					Content: as.NaturalLanguageValue{
						as.LangRefValue{Ref: as.NilLangRef, Value: "<p>Hello world</p>"},
					},
					To: as.ItemCollection{
						as.IRI("https://littr.git/api/actors/system/inbox"),
					},
					CC: as.ItemCollection{
						as.IRI("https://www.w3.org/ns/activitystreams#Public"),
					},
				},
			},
		},
	},
}

// execute Process Outgoing ActivityPub payload
var _exProcessOutgoingInboxAction = Message{
	Priority: PriorityHigh,
	Actions: []interface{}{
		APProcess{
			Actor: app.Account{
				Hash: "29fc2269252dd76fa7e4b6d193f51a3f3cd21fdf30e44f34ec138d7e803cf0c3",
				Metadata: &app.AccountMetadata{
					Key: &app.SSHKey{
						Private: []byte{0x0},
					},
				},
			},
			Activity: as.Activity{
				Parent: as.Parent{
					Type: as.CreateType,
				},
				Actor: as.IRI("https://littr.git/api/actors/system"),
				Object: &as.Object{
					Type:      as.NoteType,
					InReplyTo: as.IRI("https://external.example.com/accounts/jane_doe/outbox/special-note-identifier-123"),
					Content: as.NaturalLanguageValue{
						as.LangRefValue{Ref: as.NilLangRef, Value: "<p>The World says back: Hello Jane</p>"},
					},
					To: as.ItemCollection{
						as.IRI("https://external.example.com/accounts/jane_doe/inbox"),
					},
					CC: as.ItemCollection{
						as.IRI("https://www.w3.org/ns/activitystreams#Public"),
					},
				},
			},
		},
	},
}

// SSHKey parameters
type SSHKey struct {
	Type string   `json:"type"`
	Seed int64    `json:"seed"`
	Hash app.Hash `json:"hash"`
}

type ScoreUpdate struct {
	Type EntityType `json:"type"`
	Hash app.Hash   `json:"hash"`
}

type APProcess struct {
	Activity as.Item     `json:"activity"`
	Actor    app.Account `json:"actor"`
}

// Action can be a procedural operation, which doesn't need a Target
// or a functional operation, which does. Eg:
// {"action: { "verb": "update_score", "target": { "item": { "key" : "beef1d00de" } } }
//type Action struct {
//	Verb   ActionType    `json:"verb"`
//	Params []interface{} `json:"target,omitempty"`
//}

type QueuePriority int8

const (
	PriorityHigh QueuePriority = iota
	PriorityLow
)

//
type Message struct {
	Priority QueuePriority `json:"priority"`
	Actions  []interface{} `json:"actions"`
}

var DefaultQueue interface{} //*redismq.Queue

func InitQueues(app *app.Application) error {
	redisDb := 0
	name := "low"
	//DefaultQueue = redismq.CreateQueue(app.Config.Redis.Host, app.Config.Redis.Port, app.Config.Redis.Pw, int64(redisDb), name)
	if DefaultQueue != nil {
		app.Config.Redis.Enabled = true
	} else {
		new := errors.NewErr("unable to connect to redis")
		if len(app.Config.Redis.Host) > 0 {
			app.Logger.WithContext(log.Ctx{
				"redisHost": app.Config.Redis.Host,
				"redisPort": app.Config.Redis.Port,
				"redisDb":   redisDb,
				"name":      name,
				"trace":     new.StackTrace(),
			}).Error(new.Error())
		}
		return &new
	}
	return nil
}

func AddMessage(msg Message) (int, int, error) {
	//var which string
	//if msg.Priority == PriorityHigh {
	//	which = "high"
	//}
	//if msg.Priority == PriorityLow {
	//	which = "low"
	//}
	//if DefaultQueue == nil {
	//	return 0, 0, errors.Errorf("invalid queue name %s", which)
	//}
	//
	//processed := 0
	//erred := 0
	//for i, p := range msg.Actions {
	//	var data []byte
	//	var err error
	//	switch o := p.(type) {
	//	case SSHKey:
	//		data, err = json.Marshal(o)
	//	case ScoreUpdate:
	//		data, err = json.Marshal(o)
	//	case APProcess:
	//		data, err = jsonld.Marshal(o.Activity)
	//	}
	//	if err != nil {
	//		Logger.WithContext(log.Ctx{
	//			"queue":    which,
	//			"item":     i,
	//			"msg_cnt":  len(msg.Actions),
	//			"act_type": reflect.TypeOf(p).Name(),
	//		}).Warn(err.Error())
	//		erred++
	//		continue
	//	}
	//	if err := DefaultQueue.Put(string(data)); err == nil {
	//		processed++
	//		Logger.WithContext(log.Ctx{
	//			"queue":         which,
	//			"item":          i,
	//			"msg_cnt":       len(msg.Actions),
	//			"processed_cnt": processed,
	//			"err_cnt":       erred,
	//			"act_type":      reflect.TypeOf(p).Name(),
	//			"data":          data,
	//		}).Info("added new msg in queue")
	//	} else {
	//		erred++
	//		Logger.WithContext(log.Ctx{
	//			"queue":         which,
	//			"item":          i,
	//			"msg_cnt":       len(msg.Actions),
	//			"processed_cnt": processed,
	//			"err_cnt":       erred,
	//			"act_type":      reflect.TypeOf(p).Name(),
	//		}).Warn(err.Error())
	//	}
	//}
	return 0, 0, nil
}

func ProcessMessages(count int) (int, int, error) {
	//consumer, err := DefaultQueue.AddConsumer("consumer")
	//if err != nil {
	//	return 0, count, err
	//}
	//
	//for i := 0; i < count; i++ {
	//	pkg, err := consumer.Get()
	//	if err != nil {
	//		return 0, count, err
	//	}
	//
	//	fmt.Println(pkg.Payload)
	//}
	//
	return count, 0, errors.NotImplementedf("not implemented")
}
