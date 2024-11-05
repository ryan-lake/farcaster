package main

import (
	"context"
	"log"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/lambda/types"
)

type LambdaInfo struct {
	Name         string
	Arn          string
	LogGroup     string
	Tags         map[string]string
	LastModified string
}

type LambdaInfoWithError struct {
	LambdaInfo
	err error
}

func (li *LambdaInfo) PopulateFunctionAsync(client *lambda.Client, wg *sync.WaitGroup, resultChan chan LambdaInfoWithError) {

	trimmedArn := strings.TrimSuffix(li.Arn, ":$LATEST")
	inputWithError := LambdaInfoWithError{}
	fd, err := GetFunctionData(client, trimmedArn)
	if err != nil {
		inputWithError.err = err
		return
	}
	li.LastModified = *fd.Configuration.LastModified
	li.Tags = fd.Tags
	li.LogGroup = *fd.Configuration.LoggingConfig.LogGroup
	inputWithError.LambdaInfo = *li
	resultChan <- inputWithError
	wg.Done()
}

func GetFunctionData(client *lambda.Client, arn string) (*lambda.GetFunctionOutput, error) {

	ctx := context.Background()

	function, err := client.GetFunction(ctx, &lambda.GetFunctionInput{FunctionName: &arn})
	if err != nil {
		return nil, err
	}

	return function, nil
}

func FetchLambdaInfoAsync(wg *sync.WaitGroup, channel chan []LambdaInfo) {
	result, err := FetchLambdaInfo()
	if err != nil {
		log.Println("Error fetching lambdas")
		wg.Done()
		return
	}
	channel <- result
	wg.Done()
}

func FetchLambdaInfo() ([]LambdaInfo, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion("us-east-2"),
	)
	if err != nil {
		log.Printf("unable to load SDK config, %v", err)
		return nil, err
	}

	svc := lambda.NewFromConfig(cfg)

	log.Println("Begin fetching functions")
	functionList := []types.FunctionConfiguration{}
	result, err := svc.ListFunctions(context.TODO(), &lambda.ListFunctionsInput{FunctionVersion: "ALL"})
	if err != nil {
		log.Printf("Function list failed, %v", err)
		return nil, err
	}
	functionList = append(functionList, result.Functions...)
	marker := result.NextMarker
	for marker != nil {
		nextPage, err := svc.ListFunctions(context.TODO(), &lambda.ListFunctionsInput{Marker: marker})
		if err != nil {
			log.Printf("Function list failed at marker %s, %v", *result.NextMarker, err)
			return nil, err
		}
		functionList = append(functionList, nextPage.Functions...)
		marker = nextPage.NextMarker
	}

	log.Println("Functions fetched, beginning get Tags")
	resultChan := make(chan LambdaInfoWithError)
	var wg sync.WaitGroup

	for _, f := range functionList {
		wg.Add(1)
		li := LambdaInfo{
			Name: *f.FunctionName,
			Arn:  *f.FunctionArn,
		}
		go li.PopulateFunctionAsync(svc, &wg, resultChan)
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	functionResults := []LambdaInfo{}
	for {
		result, ok := <-resultChan
		if !ok {
			resultChan = nil
		} else {
			if result.err != nil {
				log.Printf("Function %s tags were not fetched: %v \n", result.Name, result.err)
				continue
			}
			functionResults = append(functionResults, result.LambdaInfo)
		}

		if resultChan == nil {
			break
		}
	}
	log.Println("Tags Fetched Function work done")
	return functionResults, nil
}
