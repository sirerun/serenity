package authority

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

type dynamoCapture struct {
	tx    *dynamodb.TransactWriteItemsInput
	query *dynamodb.QueryInput
}

func (d *dynamoCapture) GetItem(_ context.Context, i *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	if !aws.ToBool(i.ConsistentRead) {
		return nil, errors.New("not strong")
	}
	return &dynamodb.GetItemOutput{}, nil
}
func (d *dynamoCapture) Query(_ context.Context, i *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	d.query = i
	return &dynamodb.QueryOutput{}, nil
}
func (d *dynamoCapture) TransactWriteItems(_ context.Context, i *dynamodb.TransactWriteItemsInput, _ ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	d.tx = i
	return &dynamodb.TransactWriteItemsOutput{}, nil
}
func TestDynamoAtomicTransactionShape(t *testing.T) {
	d := new(dynamoCapture)
	s, e := NewDynamoStore(d, "table", "namespace")
	if e != nil {
		t.Fatal(e)
	}
	op := Receipt{OperationID: strings.Repeat("a", 64), RequestSHA256: strings.Repeat("b", 64), Principal: "runtime"}
	rec := Record{Revision: 8, State: json.RawMessage(`{}`), Pending: &Pending{OperationID: op.OperationID}}
	if e = s.Commit(context.Background(), 7, rec, []ImmutableItem{{Key: "OBJ#key", Body: []byte("canonical")}}, op); e != nil {
		t.Fatal(e)
	}
	for _, tx := range d.tx.TransactItems {
		if tx.Put == nil || len(tx.Put.ExpressionAttributeNames) == 0 {
			t.Fatal("expression attribute names are not aliased")
		}
	}
	if len(d.tx.TransactItems) != 3 {
		t.Fatal("state/object/receipt not atomic")
	}
	first := d.tx.TransactItems[0].Put
	if first == nil || !strings.Contains(aws.ToString(first.ConditionExpression), "#revision = :revision") || !strings.Contains(aws.ToString(first.ConditionExpression), "attribute_not_exists(#pending)") {
		t.Fatal("head epoch revision/pending fence missing")
	}
	for _, tx := range d.tx.TransactItems[1:] {
		if aws.ToString(tx.Put.ConditionExpression) != "attribute_not_exists(#pk)" {
			t.Fatal("immutable creation unconditional")
		}
	}
	if _, _, e = s.List(context.Background(), "OBJ#deletion-journal/", "", 10); e != nil {
		t.Fatal(e)
	}
	if !aws.ToBool(d.query.ConsistentRead) || aws.ToInt32(d.query.Limit) != 11 {
		t.Fatal("list not strong or bounded")
	}
}

type s3Memory struct {
	objects map[string][]byte
	etags   map[string]string
	counter int
	lose    bool
}

func newS3Memory() *s3Memory {
	return &s3Memory{objects: map[string][]byte{}, etags: map[string]string{}}
}
func (s *s3Memory) GetObject(_ context.Context, i *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	key := aws.ToString(i.Key)
	b, ok := s.objects[key]
	if !ok {
		return nil, &smithy.GenericAPIError{Code: "NoSuchKey"}
	}
	return &s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(b)), ContentLength: aws.Int64(int64(len(b))), ETag: aws.String(s.etags[key])}, nil
}
func (s *s3Memory) PutObject(_ context.Context, i *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	key := aws.ToString(i.Key)
	_, exists := s.objects[key]
	if aws.ToString(i.IfNoneMatch) == "*" && exists || i.IfMatch != nil && (!exists || aws.ToString(i.IfMatch) != s.etags[key]) {
		return nil, &smithy.GenericAPIError{Code: "PreconditionFailed"}
	}
	if i.IfNoneMatch == nil && i.IfMatch == nil {
		return nil, errors.New("unconditional write")
	}
	b, e := io.ReadAll(i.Body)
	if e != nil {
		return nil, e
	}
	s.counter++
	s.objects[key] = b
	s.etags[key] = fmt.Sprintf("etag-%d", s.counter)
	if s.lose {
		s.lose = false
		return nil, ErrUnavailable
	}
	return &s3.PutObjectOutput{ETag: aws.String(s.etags[key])}, nil
}
func TestS3WitnessConditionalReplayAndTableFence(t *testing.T) {
	api := newS3Memory()
	w, e := NewS3Witness(api, "witness-bucket", "lineage/")
	if e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	body := []byte(`{"witness":1}`)
	tip := WitnessTip{Lineage: "lineage", TableARN: "table-one", Sequence: 1, SHA256: digest(body)}
	tip.TotalBytes = int64(len(body))
	tip.Key = fmt.Sprintf("%020d/%s.json", tip.Sequence, tip.SHA256)
	p := Pending{Next: tip, Witness: body, Bootstrap: true}
	api.lose = true
	if e = w.Advance(ctx, p); e != nil {
		t.Fatalf("lost PUT not resolved: %v", e)
	}
	if e = w.Advance(ctx, p); e != nil {
		t.Fatal(e)
	}
	b2 := []byte(`{"witness":2}`)
	next := WitnessTip{Lineage: tip.Lineage, TableARN: "table-two", Sequence: 2, SHA256: digest(b2)}
	next.TotalBytes = tip.TotalBytes + int64(len(b2))
	next.Key = fmt.Sprintf("%020d/%s.json", next.Sequence, next.SHA256)
	migration := Pending{Previous: tip, Next: next, Witness: b2}
	if e = w.Advance(ctx, migration); !errors.Is(e, ErrForbidden) {
		t.Fatalf("unowned table migration: %v", e)
	}
	migration.OwnerRebind = true
	if e = w.Advance(ctx, migration); e != nil {
		t.Fatal(e)
	}
	old := migration
	old.OwnerRebind = false
	old.Next.TableARN = tip.TableARN
	if e = w.Advance(ctx, old); !errors.Is(e, ErrConflict) {
		t.Fatalf("retired table rewrote tip: %v", e)
	}
}
func TestGatewayRejectsForgedContextAndOversizedBody(t *testing.T) {
	s, _, _, _, _, _ := activeFixture(t)
	runtime := "arn:aws:sts::123456789012:assumed-role/runtime/instance"
	owner := "arn:aws:iam::123456789012:user/owner"
	s.config.RuntimePrincipal = runtime
	s.config.OwnerPrincipal = owner
	b := GatewayBinding{APIID: "api", AccountID: "123456789012", Stage: "prod", DomainName: "api.execute-api.eu-west-1.amazonaws.com", RuntimePath: "/runtime", OwnerPath: "/owner", RuntimeARN: runtime, RuntimeCallerID: "ROLE:instance", OwnerARN: owner, OwnerCallerID: "OWNER"}
	g, e := NewGateway(s, b)
	if e != nil {
		t.Fatal(e)
	}
	good := events.APIGatewayProxyRequest{Path: "/runtime", Resource: "/runtime", HTTPMethod: "POST", Body: `{"action":"current","lease_id":"` + strings.Repeat("a", 64) + `"}`, RequestContext: events.APIGatewayProxyRequestContext{APIID: b.APIID, AccountID: b.AccountID, Stage: b.Stage, DomainName: b.DomainName, HTTPMethod: "POST", RequestID: "request", ResourcePath: b.RuntimePath, Identity: events.APIGatewayRequestIdentity{UserArn: runtime, Caller: b.RuntimeCallerID, AccessKey: ""}}}
	// REST AWS_IAM context documents caller and userArn; accessKey is optional.
	ownerEvent := good
	ownerEvent.Path, ownerEvent.Resource = b.OwnerPath, b.OwnerPath
	ownerEvent.RequestContext.ResourcePath = b.OwnerPath
	ownerEvent.RequestContext.Identity = events.APIGatewayRequestIdentity{UserArn: owner, Caller: b.OwnerCallerID}
	ownerEvent.Body = `{"action":"inspect"}`
	accepted, err := g.Handle(context.Background(), ownerEvent)
	if err != nil || accepted.StatusCode != 200 {
		t.Fatalf("authenticated owner without accessKey refused: %d %v", accepted.StatusCode, err)
	}
	tests := map[string]func(*events.APIGatewayProxyRequest){"wrong api": func(e *events.APIGatewayProxyRequest) { e.RequestContext.APIID = "other" }, "wrong account": func(e *events.APIGatewayProxyRequest) { e.RequestContext.AccountID = "other" }, "wrong principal": func(e *events.APIGatewayProxyRequest) { e.RequestContext.Identity.UserArn = owner }, "wrong caller": func(e *events.APIGatewayProxyRequest) { e.RequestContext.Identity.Caller = "attacker" }, "empty principal": func(e *events.APIGatewayProxyRequest) { e.RequestContext.Identity.UserArn = "" }, "empty caller": func(e *events.APIGatewayProxyRequest) { e.RequestContext.Identity.Caller = "" }, "wrong resource": func(e *events.APIGatewayProxyRequest) { e.Resource = "/owner" }, "wrong context path": func(e *events.APIGatewayProxyRequest) { e.RequestContext.ResourcePath = "/owner" }, "wrong stage": func(e *events.APIGatewayProxyRequest) { e.RequestContext.Stage = "other" }, "wrong domain": func(e *events.APIGatewayProxyRequest) { e.RequestContext.DomainName = "other" }, "owner path with runtime": func(e *events.APIGatewayProxyRequest) { e.Path = "/owner" }, "authorizer spoof": func(e *events.APIGatewayProxyRequest) {
		e.RequestContext.Authorizer = map[string]interface{}{"principalId": "owner"}
	}}
	for name, modify := range tests {
		t.Run(name, func(t *testing.T) {
			event := good
			modify(&event)
			r, e := g.Handle(context.Background(), event)
			if e != nil || r.StatusCode != 403 {
				t.Fatalf("spoof accepted: %d %v", r.StatusCode, e)
			}
		})
	}
	good.Body = strings.Repeat("x", MaxBodyBytes+1)
	r, e := g.Handle(context.Background(), good)
	if e != nil || r.StatusCode != 400 {
		t.Fatalf("oversize accepted: %d %v", r.StatusCode, e)
	}
}

func TestGatewayRejectionDiagnosticsContainOnlyBooleans(t *testing.T) {
	var out bytes.Buffer
	oldOutput, oldFlags := log.Writer(), log.Flags()
	log.SetOutput(&out)
	log.SetFlags(0)
	defer func() { log.SetOutput(oldOutput); log.SetFlags(oldFlags) }()
	g := Gateway{binding: GatewayBinding{APIID: "configured-api-secret", RuntimePath: "/runtime", OwnerPath: "/owner"}}
	g.logRejectedContext(events.APIGatewayProxyRequest{
		Path: "request-path-secret", Body: "request-body-secret",
		Headers: map[string]string{"Authorization": "credential-secret"},
		RequestContext: events.APIGatewayProxyRequestContext{
			APIID:    "request-api-secret",
			Identity: events.APIGatewayRequestIdentity{UserArn: "principal-secret", Caller: "caller-secret", AccessKey: "access-key-secret"},
		},
	})
	want := "authority gateway rejected context: api=true account=false stage=false domain=false method=true request_id=true authorizer=false route=true principal=true caller=true\n"
	if out.String() != want {
		t.Fatalf("unexpected diagnostic shape: %q", out.String())
	}
}
