package authority

import (
	"bytes"
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
	r, err := d.api.GetItem(ctx, &dynamodb.GetItemInput{TableName: aws.String(d.table), Key: d.key(key), ConsistentRead: aws.Bool(true), ProjectionExpression: aws.String("#payload"), ExpressionAttributeNames: map[string]string{"#payload": "payload"}})
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
	cond := "attribute_not_exists(#pk)"
	names := map[string]string{"#pk": "pk"}
	var values map[string]types.AttributeValue
	if expected > 0 {
		cond = "#revision = :revision AND attribute_not_exists(#pending)"
		names = map[string]string{"#revision": "revision", "#pending": "pending"}
		values = map[string]types.AttributeValue{":revision": &types.AttributeValueMemberN{Value: strconv.FormatUint(expected, 10)}}
	}
	tx := []types.TransactWriteItem{{Put: &types.Put{TableName: aws.String(d.table), Item: meta, ConditionExpression: aws.String(cond), ExpressionAttributeValues: values, ExpressionAttributeNames: names}}}
	op, err := d.item("OP#"+r.OperationID, r)
	if err != nil {
		return err
	}
	tx = append(tx, types.TransactWriteItem{Put: &types.Put{TableName: aws.String(d.table), Item: op, ConditionExpression: aws.String("attribute_not_exists(#pk)"), ExpressionAttributeNames: map[string]string{"#pk": "pk"}}})
	seen := map[string]bool{"META": true, "OP#" + r.OperationID: true}
	for _, item := range items {
		if seen[item.Key] || item.Key == "" || strings.HasPrefix(item.Key, "OP#") {
			return ErrInvalid
		}
		seen[item.Key] = true
		m := d.key(item.Key)
		m["payload"] = &types.AttributeValueMemberB{Value: item.Body}
		tx = append(tx, types.TransactWriteItem{Put: &types.Put{TableName: aws.String(d.table), Item: m, ConditionExpression: aws.String("attribute_not_exists(#pk)"), ExpressionAttributeNames: map[string]string{"#pk": "pk"}}})
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
		{Put: &types.Put{TableName: aws.String(d.table), Item: meta, ConditionExpression: aws.String("#revision = :revision AND #pending = :pending"), ExpressionAttributeNames: map[string]string{"#revision": "revision", "#pending": "pending"}, ExpressionAttributeValues: map[string]types.AttributeValue{":revision": &types.AttributeValueMemberN{Value: strconv.FormatUint(r.Revision, 10)}, ":pending": &types.AttributeValueMemberS{Value: p.OperationID}}}},
		{Put: &types.Put{TableName: aws.String(d.table), Item: op, ConditionExpression: aws.String("attribute_exists(#pk)"), ExpressionAttributeNames: map[string]string{"#pk": "pk"}}},
	}})
	return err
}
func (d *DynamoStore) List(ctx context.Context, prefix, after string, limit int) ([]string, bool, error) {
	objects, more, err := d.ListObjects(ctx, prefix, after, limit)
	if err != nil {
		return nil, false, err
	}
	keys := make([]string, len(objects))
	for i, o := range objects {
		keys[i] = o.Key
	}
	return keys, more, nil
}

// ListObjects projects canonical payloads in the same strongly consistent
// Query page. AWS bounds a Query page at1MiB; the outer protocol further bounds
// the encoded response before sending any bytes to the runtime.
func (d *DynamoStore) ListObjects(ctx context.Context, prefix, after string, limit int) ([]ImmutableItem, bool, error) {
	if limit < 1 || limit > MaxPageKeys || len(prefix) > 512 || len(after) > 512 {
		return nil, false, ErrInvalid
	}
	in := &dynamodb.QueryInput{TableName: aws.String(d.table), ConsistentRead: aws.Bool(true), KeyConditionExpression: aws.String("#pk = :pk AND begins_with(#sk, :prefix)"), ExpressionAttributeNames: map[string]string{"#pk": "pk", "#sk": "sk", "#payload": "payload"}, ExpressionAttributeValues: map[string]types.AttributeValue{":pk": &types.AttributeValueMemberS{Value: d.namespace}, ":prefix": &types.AttributeValueMemberS{Value: prefix}}, ProjectionExpression: aws.String("#sk, #payload"), ScanIndexForward: aws.Bool(true), Limit: aws.Int32(int32(limit + 1))}
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
	objects := make([]ImmutableItem, 0, len(r.Items))
	last := after
	for _, item := range r.Items {
		key, ok := item["sk"].(*types.AttributeValueMemberS)
		if !ok || !strings.HasPrefix(key.Value, prefix) || key.Value <= last {
			return nil, false, fmt.Errorf("query order: %w", ErrInvalid)
		}
		body, ok := item["payload"].(*types.AttributeValueMemberB)
		if !ok || len(body.Value) > MaxObjectBytes {
			return nil, false, ErrInvalid
		}
		objects = append(objects, ImmutableItem{Key: key.Value, Body: append([]byte(nil), body.Value...)})
		last = key.Value
	}
	more := len(r.LastEvaluatedKey) > 0 || len(objects) > limit
	if len(objects) > limit {
		objects = objects[:limit]
	}
	return objects, more, nil
}

// Import is used only while owner recovery has not published META. It never
// overwrites an existing item; an interrupted batch resumes by exact readback.
func (d *DynamoStore) Import(ctx context.Context, items []ImmutableItem) error {
	for len(items) > 0 {
		var batch []ImmutableItem
		size := 0
		for len(items) > 0 && len(batch) < 80 {
			it := items[0]
			if it.Key == "META" || it.Key == "" || len(it.Key) > 512 || len(it.Body) > 350*1024 {
				return ErrInvalid
			}
			if size+len(it.Body) > 3*1024*1024 && len(batch) > 0 {
				break
			}
			items = items[1:]
			stored, found, err := d.Get(ctx, it.Key)
			if err != nil {
				return err
			}
			if found {
				if !bytes.Equal(stored, it.Body) {
					return ErrConflict
				}
				continue
			}
			batch = append(batch, it)
			size += len(it.Body)
		}
		if len(batch) == 0 {
			continue
		}
		tx := make([]types.TransactWriteItem, 0, len(batch))
		for _, it := range batch {
			item := d.key(it.Key)
			item["payload"] = &types.AttributeValueMemberB{Value: it.Body}
			tx = append(tx, types.TransactWriteItem{Put: &types.Put{TableName: aws.String(d.table), Item: item, ConditionExpression: aws.String("attribute_not_exists(#pk)"), ExpressionAttributeNames: map[string]string{"#pk": "pk"}}})
		}
		if _, err := d.api.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: tx}); err != nil {
			for _, it := range batch {
				stored, found, readErr := d.Get(ctx, it.Key)
				if readErr != nil || !found {
					return ErrUnavailable
				}
				if !bytes.Equal(stored, it.Body) {
					return ErrConflict
				}
			}
		}
	}
	return nil
}
