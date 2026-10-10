package authority

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type DynamoAPI interface {
	GetItem(context.Context, *dynamodb.GetItemInput, ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	Query(context.Context, *dynamodb.QueryInput, ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	TransactWriteItems(context.Context, *dynamodb.TransactWriteItemsInput, ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error)
}

type DynamoStore struct {
	api              DynamoAPI
	table, namespace string
}

// NewDynamoStore accepts an explicitly credentialed SDK client. It never loads
// credentials, profiles, environment configuration, or SDK defaults.
func NewDynamoStore(api DynamoAPI, table, namespace string) (*DynamoStore, error) {
	if api == nil || table == "" || namespace == "" || len(namespace) > 256 {
		return nil, ErrInvalid
	}
	return &DynamoStore{api: api, table: table, namespace: namespace}, nil
}
func (d *DynamoStore) key(key string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{"pk": &types.AttributeValueMemberS{Value: d.namespace}, "sk": &types.AttributeValueMemberS{Value: key}}
}
func (d *DynamoStore) Get(ctx context.Context, key string) ([]byte, bool, error) {
	r, err := d.api.GetItem(ctx, &dynamodb.GetItemInput{TableName: aws.String(d.table), Key: d.key(key), ConsistentRead: aws.Bool(true), ProjectionExpression: aws.String("payload")})
	if err != nil {
		return nil, false, err
	}
	if r == nil {
		return nil, false, ErrUnavailable
	}
	if len(r.Item) == 0 {
		return nil, false, nil
	}
	b, ok := r.Item["payload"].(*types.AttributeValueMemberB)
	if !ok || len(b.Value) > 350*1024 {
		return nil, false, ErrInvalid
	}
	return append([]byte(nil), b.Value...), true, nil
}
func (d *DynamoStore) Load(ctx context.Context) (Record, bool, error) {
	b, found, err := d.Get(ctx, "META")
	if err != nil || !found {
		return Record{}, found, err
	}
	var r Record
	if err = json.Unmarshal(b, &r); err != nil {
		return Record{}, false, err
	}
	return r, true, nil
}
func (d *DynamoStore) Receipt(ctx context.Context, id string) (Receipt, bool, error) {
	if !validID(id) {
		return Receipt{}, false, ErrInvalid
	}
	b, found, err := d.Get(ctx, "OP#"+id)
	if err != nil || !found {
		return Receipt{}, found, err
	}
	var r Receipt
	if err = json.Unmarshal(b, &r); err != nil {
		return Receipt{}, false, err
	}
	return r, true, nil
}
func (d *DynamoStore) item(key string, v any) (map[string]types.AttributeValue, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	if len(b) > 350*1024 {
		return nil, ErrInvalid
	}
	m := d.key(key)
	m["payload"] = &types.AttributeValueMemberB{Value: b}
	return m, nil
}
func (d *DynamoStore) Commit(ctx context.Context, expected uint64, next Record, items []ImmutableItem, r Receipt) error {
	meta, err := d.item("META", next)
	if err != nil {
		return err
	}
	meta["revision"] = &types.AttributeValueMemberN{Value: strconv.FormatUint(next.Revision, 10)}
	meta["pending"] = &types.AttributeValueMemberS{Value: r.OperationID}
	cond := "attribute_not_exists(pk)"
	var values map[string]types.AttributeValue
	if expected > 0 {
		cond = "revision = :revision AND attribute_not_exists(pending)"
		values = map[string]types.AttributeValue{":revision": &types.AttributeValueMemberN{Value: strconv.FormatUint(expected, 10)}}
	}
	tx := []types.TransactWriteItem{{Put: &types.Put{TableName: aws.String(d.table), Item: meta, ConditionExpression: aws.String(cond), ExpressionAttributeValues: values}}}
	op, err := d.item("OP#"+r.OperationID, r)
	if err != nil {
		return err
	}
	tx = append(tx, types.TransactWriteItem{Put: &types.Put{TableName: aws.String(d.table), Item: op, ConditionExpression: aws.String("attribute_not_exists(pk)")}})
	seen := map[string]bool{"META": true, "OP#" + r.OperationID: true}
	for _, item := range items {
		if seen[item.Key] || item.Key == "" || strings.HasPrefix(item.Key, "OP#") {
			return ErrInvalid
		}
		seen[item.Key] = true
		m := d.key(item.Key)
		m["payload"] = &types.AttributeValueMemberB{Value: item.Body}
		tx = append(tx, types.TransactWriteItem{Put: &types.Put{TableName: aws.String(d.table), Item: m, ConditionExpression: aws.String("attribute_not_exists(pk)")}})
	}
	if len(tx) > 100 {
		return ErrInvalid
	}
	_, err = d.api.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: tx})
	return err
}
func (d *DynamoStore) Complete(ctx context.Context, r Record) error {
	if r.Pending == nil {
		return ErrInvalid
	}
	p := r.Pending
	r.Tip = p.Next
	r.Pending = nil
	meta, err := d.item("META", r)
	if err != nil {
		return err
	}
	meta["revision"] = &types.AttributeValueMemberN{Value: strconv.FormatUint(r.Revision, 10)}
	receipt := Receipt{OperationID: p.OperationID, RequestSHA256: p.RequestSHA256, Principal: p.Principal, Response: p.Response, Complete: true}
	op, err := d.item("OP#"+p.OperationID, receipt)
	if err != nil {
		return err
	}
	_, err = d.api.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: []types.TransactWriteItem{
		{Put: &types.Put{TableName: aws.String(d.table), Item: meta, ConditionExpression: aws.String("revision = :revision AND pending = :pending"), ExpressionAttributeValues: map[string]types.AttributeValue{":revision": &types.AttributeValueMemberN{Value: strconv.FormatUint(r.Revision, 10)}, ":pending": &types.AttributeValueMemberS{Value: p.OperationID}}}},
		{Put: &types.Put{TableName: aws.String(d.table), Item: op, ConditionExpression: aws.String("attribute_exists(pk)")}},
	}})
	return err
}
func (d *DynamoStore) List(ctx context.Context, prefix, after string, limit int) ([]string, bool, error) {
	if limit < 1 || limit > MaxPageKeys || len(prefix) > 512 || len(after) > 512 {
		return nil, false, ErrInvalid
	}
	in := &dynamodb.QueryInput{TableName: aws.String(d.table), ConsistentRead: aws.Bool(true), KeyConditionExpression: aws.String("pk = :pk AND begins_with(sk, :prefix)"), ExpressionAttributeValues: map[string]types.AttributeValue{":pk": &types.AttributeValueMemberS{Value: d.namespace}, ":prefix": &types.AttributeValueMemberS{Value: prefix}}, ProjectionExpression: aws.String("sk"), ScanIndexForward: aws.Bool(true), Limit: aws.Int32(int32(limit + 1))}
	if after != "" {
		if !strings.HasPrefix(after, prefix) {
			return nil, false, ErrInvalid
		}
		in.ExclusiveStartKey = d.key(after)
	}
	r, err := d.api.Query(ctx, in)
	if err != nil {
		return nil, false, err
	}
	if r == nil {
		return nil, false, ErrUnavailable
	}
	keys := make([]string, 0, len(r.Items))
	last := after
	for _, item := range r.Items {
		v, ok := item["sk"].(*types.AttributeValueMemberS)
		if !ok || !strings.HasPrefix(v.Value, prefix) || v.Value <= last {
			return nil, false, fmt.Errorf("query order: %w", ErrInvalid)
		}
		keys = append(keys, v.Value)
		last = v.Value
	}
	more := len(r.LastEvaluatedKey) > 0 || len(keys) > limit
	if len(keys) > limit {
		keys = keys[:limit]
	}
	return keys, more, nil
}
