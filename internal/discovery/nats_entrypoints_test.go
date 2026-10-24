package discovery_test

import (
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/DiLRandI/OTelPlan/internal/discovery"
	"github.com/DiLRandI/OTelPlan/pkg/model"
)

func TestDiscoveryIndexesTypedNATSEntrypoints(t *testing.T) {
	t.Parallel()

	root := natsEntrypointFixture(t)
	first := loadNATSEntrypointModel(t, root)
	second := loadNATSEntrypointModel(t, root)

	want := map[model.SymbolID][]string{
		"example.com/natsentrypoints.(Implementation).Core": {
			"github.com/nats-io/nats.go.(*Conn).Subscribe",
		},
		"example.com/natsentrypoints.(Implementation).Queue": {
			"github.com/nats-io/nats.go.(*Conn).QueueSubscribe",
		},
		"example.com/natsentrypoints.(Implementation).Legacy": {
			"github.com/nats-io/nats.go.(JetStream).Subscribe",
		},
		"example.com/natsentrypoints.(Implementation).LegacyQueue": {
			"github.com/nats-io/nats.go.(JetStream).QueueSubscribe",
		},
		"example.com/natsentrypoints.(Implementation).Pull": {
			"github.com/nats-io/nats.go/jetstream.(Consumer).Consume",
		},
		"example.com/natsentrypoints.(Implementation).Push": {
			"github.com/nats-io/nats.go/jetstream.(PushConsumer).Consume",
		},
		"example.com/natsentrypoints.(Embedded).Promoted": {
			"github.com/nats-io/nats.go.(*Conn).Subscribe",
		},
	}

	for symbolID, entrypoints := range want {
		assertNATSEntrypoints(t, first, symbolID, entrypoints)
		assertNATSEntrypoints(t, second, symbolID, entrypoints)
	}

	for _, symbolID := range []model.SymbolID{
		"example.com/natsentrypoints.Unregistered",
		"example.com/natsentrypoints.Stored",
		"example.com/natsentrypoints.FakeTarget",
		"example.com/natsentrypoints.OnError",
		"example.com/natsentrypoints.Anonymous",
	} {
		assertNATSEntrypoints(t, first, symbolID, nil)
	}

	if !reflect.DeepEqual(first.Symbols, second.Symbols) {
		t.Fatal("NATS entrypoint metadata was not deterministic")
	}

	encoded, err := json.Marshal(first.Symbols)
	if err != nil {
		t.Fatal(err)
	}

	for _, privateValue := range []string{"private.subject", "private.queue"} {
		if strings.Contains(string(encoded), privateValue) {
			t.Fatalf("NATS registration value leaked into symbol metadata: %s", privateValue)
		}
	}
}

func assertNATSEntrypoints(t *testing.T, code *model.CodeModel, symbolID model.SymbolID, want []string) {
	t.Helper()

	symbol, found := code.Symbol(symbolID)
	if !found {
		t.Fatalf("missing NATS fixture symbol %s", symbolID)
	}

	if !slices.Equal(symbol.Entrypoints, want) {
		t.Fatalf("NATS entrypoints for %s = %v, want %v", symbolID, symbol.Entrypoints, want)
	}
}

func loadNATSEntrypointModel(t *testing.T, root string) *model.CodeModel {
	t.Helper()

	var options discovery.Options

	options.Root, options.Offline = root, true
	options.Env = []string{"GOWORK=off", "GOFLAGS=", "CGO_ENABLED=0"}

	code, err := discovery.LoadContext(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}

	return code
}

func TestNATSEntrypointCacheAndRelocation(t *testing.T) {
	t.Parallel()

	root := natsEntrypointFixture(t)

	var options discovery.Options

	options.Root, options.Offline, options.CacheDir = root, true, t.TempDir()
	options.Env = []string{"GOWORK=off", "GOFLAGS=", "CGO_ENABLED=0"}

	for _, readOnly := range []bool{false, true} {
		options.CacheReadOnly = readOnly

		code, err := discovery.LoadContext(t.Context(), options)
		if err != nil {
			t.Fatal(err)
		}

		assertNATSEntrypoints(t, code, "example.com/natsentrypoints.(Implementation).Pull",
			[]string{"github.com/nats-io/nats.go/jetstream.(Consumer).Consume"})
		assertNATSEntrypoints(t, code, "example.com/natsentrypoints.Stored", nil)
	}

	relocated := t.TempDir()

	err := os.CopyFS(relocated, os.DirFS(root))
	if err != nil {
		t.Fatal(err)
	}

	code := loadNATSEntrypointModel(t, relocated)
	assertNATSEntrypoints(t, code, "example.com/natsentrypoints.(Implementation).Pull",
		[]string{"github.com/nats-io/nats.go/jetstream.(Consumer).Consume"})
}

func natsEntrypointFixture(t *testing.T) string {
	t.Helper()

	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		closeErr := root.Close()
		if closeErr != nil {
			t.Errorf("close NATS entrypoint fixture: %v", closeErr)
		}
	})

	files := map[string]string{
		"go.mod": `module example.com/natsentrypoints

go 1.27

require github.com/nats-io/nats.go v0.0.0

replace github.com/nats-io/nats.go => ./nats
`,
		"nats/go.mod": `module github.com/nats-io/nats.go

go 1.27
`,
		"nats/nats.go": `package nats

type Msg struct{}
type MsgHandler func(*Msg)
type SubOpt interface{}
type Subscription struct{}
type ErrHandler func(*Conn, *Subscription, error)

type Conn struct{}

func (*Conn) Subscribe(string, MsgHandler) (*Subscription, error) { return nil, nil }
func (*Conn) QueueSubscribe(string, string, MsgHandler) (*Subscription, error) {
	return nil, nil
}
func (*Conn) SetErrorHandler(ErrHandler) {}

type JetStream interface {
	Subscribe(string, MsgHandler, ...SubOpt) (*Subscription, error)
	QueueSubscribe(string, string, MsgHandler, ...SubOpt) (*Subscription, error)
}
`,
		"nats/jetstream/jetstream.go": `package jetstream

type Msg interface{}
type MessageHandler func(Msg)
type PullConsumeOpt interface{}
type PushConsumeOpt interface{}
type ConsumeContext interface{}

type Consumer interface {
	Consume(MessageHandler, ...PullConsumeOpt) (ConsumeContext, error)
}

type PushConsumer interface {
	Consume(MessageHandler, ...PushConsumeOpt) (ConsumeContext, error)
}

`,
		"app.go": `package natsentrypoints

import (
	"github.com/nats-io/nats.go"
	natsalias "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	jetalias "github.com/nats-io/nats.go/jetstream"
)

type Implementation struct{}

func (Implementation) Core(*nats.Msg) {}
func (Implementation) Queue(*nats.Msg) {}
func (Implementation) Legacy(*nats.Msg) {}
func (Implementation) LegacyQueue(*nats.Msg) {}
func (Implementation) Pull(jetstream.Msg) {}
func (Implementation) Push(jetstream.Msg) {}

type Embedded struct{}
func (Embedded) Promoted(*nats.Msg) {}
type Promoted struct{ Embedded }

type jetstreamConsumer struct{}
func (jetstreamConsumer) Consume(
	jetstream.MessageHandler, ...jetstream.PullConsumeOpt,
) (jetstream.ConsumeContext, error) {
	return nil, nil
}

type pushConsumer struct{}
func (pushConsumer) Consume(jetstream.MessageHandler, ...jetstream.PushConsumeOpt) (jetstream.ConsumeContext, error) {
	return nil, nil
}

func Unregistered(*nats.Msg) {}
func Stored(*nats.Msg) {}
var stored = Stored

type Fake struct{}
func (Fake) Subscribe(string, nats.MsgHandler) (*nats.Subscription, error) { return nil, nil }
func FakeTarget(*nats.Msg) {}

func OnError(*nats.Conn, *nats.Subscription, error) {}
func Anonymous(*nats.Msg) {}

type jetStream struct{}
func (jetStream) Subscribe(string, nats.MsgHandler, ...nats.SubOpt) (*nats.Subscription, error) { return nil, nil }
func (jetStream) QueueSubscribe(
	subject, queue string, handler nats.MsgHandler, opts ...nats.SubOpt,
) (*nats.Subscription, error) {
	return nil, nil
}

func register() {
	conn := new(nats.Conn)
	implementation := Implementation{}
	conn.Subscribe("private.subject", implementation.Core)
	conn.QueueSubscribe("private.subject", "private.queue", implementation.Queue)
	(*nats.Conn).Subscribe(conn, "private.subject", nats.MsgHandler(implementation.Core))
	conn.Subscribe("private.subject", natsalias.MsgHandler(implementation.Core))
	conn.Subscribe("private.subject", stored)
	conn.Subscribe("private.subject", func(message *nats.Msg) { Anonymous(message) })

	var legacy nats.JetStream = jetStream{}
	legacy.Subscribe("private.subject", implementation.Legacy, nats.SubOpt(nil))
	legacy.QueueSubscribe("private.subject", "private.queue", implementation.LegacyQueue)

	var consumer jetstream.Consumer = jetstreamConsumer{}
	consumer.Consume(implementation.Pull, jetstream.PullConsumeOpt(nil), jetstream.PullConsumeOpt(nil))
	jetstream.Consumer.Consume(consumer, implementation.Pull, jetstream.PullConsumeOpt(nil))
	var push jetalias.PushConsumer = pushConsumer{}
	push.Consume(implementation.Push, jetstream.PushConsumeOpt(nil))

	promoted := Promoted{}
	conn.Subscribe("private.subject", promoted.Promoted)

	Fake{}.Subscribe("private.subject", FakeTarget)
	conn.SetErrorHandler(OnError)
}
`,
	}

	err = root.MkdirAll("nats/jetstream", 0o700)
	if err != nil {
		t.Fatal(err)
	}

	for name, contents := range files {
		err = root.WriteFile(name, []byte(contents), 0o600)
		if err != nil {
			t.Fatal(err)
		}
	}

	return root.Name()
}
