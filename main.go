package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"log"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/open-policy-agent/opa/rego"
)

type BucketBasics struct {
	S3Client *s3.Client
}

const (
	AWS_S3_REGION = "us-west-2" // Region
)

var nameToContent = make(map[string]string)
var name string
var st []string
var input map[string]interface{}

// evaluate the rego rule
func regoParser(input map[string]interface{}, b string) bool {

	var pkg string
	lines := strings.Split(b, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "package") {
			pkg = strings.TrimSpace(strings.TrimPrefix(line, "package"))
			break
		}
	}
	rule := "data." + pkg + ".allow"
	// Parse the Rego file
	r := rego.New(
		rego.Query(rule),
		rego.Module("all_rules", b),
		rego.Input(input),
	)
	rs, err := r.Eval(context.Background())
	if err != nil {
		fmt.Println("Error evaluating Rego file:", err)
		return false
	}
	if len(rs) == 0 {
		return false
	} else {
		if rs[0].Expressions[0].Value == true {
			return true
		} else if rs[0].Expressions[0].Value == false {
			return false
		}
	}
	return false
}

// evaluate the rego rules in the bucket
func evaluatePolicies(w http.ResponseWriter, r *http.Request) {

	result := true

	for i := 0; i < len(st); i++ {
		result = result && regoParser(input, nameToContent[st[i]])

		if result == false {
			break
		}
	}

	if result == true {
		fmt.Fprintf(w, "allow==true")
	} else {
		fmt.Fprintf(w, "allow==false")
	}

}

// DownloadFile gets an object from a bucket and stores it in a local file.
func (basics BucketBasics) DownloadFile(bucketName string, objectKey string) (string, error) {
	result, err := basics.S3Client.GetObject(context.TODO(), &s3.GetObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		log.Printf("Couldn't get object %v:%v. Here's why: %v\n", bucketName, objectKey, err)
		return "", err
	}
	defer result.Body.Close()
	body, err := io.ReadAll(result.Body)
	ans := string(body)
	return ans, nil
}

func handleRequest(w http.ResponseWriter, r *http.Request) {
	name = r.URL.Path[12:]
	f := false
	_, ok := nameToContent[name]
	if ok {
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "")
		fmt.Fprintf(w, "%s", nameToContent[name])
		f = true
	}

	if f == false {
		fmt.Fprintf(w, "The file is not present in bucket!!!!OOPS!!!!")
	}
	return
}

func showfiles(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "Files in bucket are: ")
	for i := 0; i < len(st); i++ {
		fmt.Fprintf(w, "%s", st[i])
		fmt.Fprintf(w, "\n")
	}
}

func main() {

	// Read the contents of the input.json file
	input = map[string]interface{}{}

	inputFile, err := os.Open("input.json")
	if err != nil {
		log.Fatalf("Error opening input.json: %v", err)
	}

	if err := json.NewDecoder(inputFile).Decode(&input); err != nil {
		log.Fatalf("Error decoding input.json: %v", err)
	}

	// Load the SDK's configuration from environment and shared config, and
	// create the client with this.
	cfg, err := config.LoadDefaultConfig(context.TODO(),

		config.WithRegion("us-west-2"))
	if err != nil {
		log.Fatalf("failed to load SDK configuration, %v", err)
	}

	s3Client := s3.NewFromConfig(cfg)

	output, er := s3Client.ListObjectsV2(context.TODO(), &s3.ListObjectsV2Input{
		Bucket: aws.String("intern-averlon-rego"),
	})
	if er != nil {
		fmt.Println("error occured")
	}
	for _, obj := range output.Contents {
		st = append(st, *obj.Key)
	}

	bucketBasics := BucketBasics{S3Client: s3Client}

	//download the content of each file into a map object
	for j := 0; j < len(st); j++ {
		cnt, _ := bucketBasics.DownloadFile("intern-averlon-rego", st[j])
		nameToContent[st[j]] = cnt
	}

	handler := http.HandlerFunc(handleRequest)
	http.Handle("/averlon/s3/", handler)
	http.HandleFunc("/averlon/s3", showfiles)
	http.HandleFunc("/averlon/s3/evaluatePolicies", evaluatePolicies)
	http.ListenAndServe(":8081", nil)
}
