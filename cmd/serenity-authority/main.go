// Command serenity-authority is the pinned AWS Lambda journal gateway.
package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/sirerun/serenity/internal/hosted/authority"
)

func gateway() (*authority.Gateway, error) {
	get := func(k string) string { return strings.TrimSpace(os.Getenv(k)) }
	region := get("AWS_REGION")
	access, secret, token := get("AWS_ACCESS_KEY_ID"), get("AWS_SECRET_ACCESS_KEY"), get("AWS_SESSION_TOKEN")
	if region == "" || access == "" || secret == "" || token == "" || get("AWS_LAMBDA_FUNCTION_VERSION") == "" || get("AWS_LAMBDA_FUNCTION_VERSION") == "$LATEST" {
		return nil, errors.New("authority requires a published Lambda version and runtime session credentials")
	}
	// Lambda supplies this session. Do not consult profiles, web identity, process
	// credentials, endpoint environment variables, or default provider chains.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	cfg := aws.Config{Region: region, Credentials: credentials.NewStaticCredentialsProvider(access, secret, token),
		HTTPClient: &http.Client{Transport: transport, Timeout: 8 * time.Second}, RetryMaxAttempts: 2}
	table, namespace, lineage := get("AUTHORITY_TABLE"), get("AUTHORITY_NAMESPACE"), get("AUTHORITY_LINEAGE")
	store, err := authority.NewDynamoStore(dynamodb.NewFromConfig(cfg), table, namespace)
	if err != nil {
		return nil, err
	}
	witness, err := authority.NewS3Witness(s3.NewFromConfig(cfg), get("WITNESS_BUCKET"), "journal/")
	if err != nil {
		return nil, err
	}
	var builds map[string]string
	if err := json.Unmarshal([]byte(get("APPROVED_BUILDS")), &builds); err != nil {
		return nil, err
	}
	runtimeARN, ownerARN := get("RUNTIME_PRINCIPAL"), get("OWNER_PRINCIPAL")
	server, err := authority.NewServer(&authority.Kernel{Store: store, Witness: witness, Lineage: lineage, TableARN: get("AUTHORITY_TABLE_ARN")},
		authority.ServerConfig{RuntimePrincipal: runtimeARN, OwnerPrincipal: ownerARN, ApprovedBuilds: builds,
			LeaseLifetime: time.Hour, ChallengeLifetime: time.Minute, GenesisLifetime: time.Hour})
	if err != nil {
		return nil, err
	}
	return authority.NewGateway(server, authority.GatewayBinding{
		APIID: get("API_ID"), AccountID: get("ACCOUNT_ID"), Stage: "prod", DomainName: get("API_DOMAIN"),
		RuntimePath: "/runtime", OwnerPath: "/owner", RuntimeARN: runtimeARN, OwnerARN: ownerARN,
		RuntimeCallerID: get("RUNTIME_CALLER_ID"), OwnerCallerID: get("OWNER_CALLER_ID")})
}

func main() {
	handler, err := gateway()
	if err != nil {
		log.Fatal("authority deployment configuration invalid")
	}
	lambda.Start(handler.Handle)
}
