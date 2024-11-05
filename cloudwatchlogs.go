package main

import (
	"context"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
)

func BeginLiveStream(groupName string) {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		log.Fatal("Configuration error in cloudwatch logs:", err.Error())
	}
	client := cloudwatchlogs.NewFromConfig(cfg)

	arn, err := getArnForGroup(client, groupName)
	if err != nil {
		log.Fatal("Requested loggroup not found")
	}
	request := &cloudwatchlogs.StartLiveTailInput{
		LogGroupIdentifiers: []string{arn},
	}
	fmt.Println(request)

	response, err := client.StartLiveTail(context.TODO(), request)
	if err != nil {
		log.Fatalf("Failed to start logstream on %v: %v", groupName, err)
	}

	stream := response.GetStream()
	go handleEventStreamAsync(stream)
}

func getArnForGroup(client *cloudwatchlogs.Client, groupName string) (string, error) {
	response, err := client.DescribeLogGroups(context.TODO(), &cloudwatchlogs.DescribeLogGroupsInput{LogGroupNamePrefix: &groupName})
	if err != nil {
		log.Println("Error fetching log group arn")
		return "", err
	}
	return *response.LogGroups[0].LogGroupArn, nil
}

func handleEventStreamAsync(stream *cloudwatchlogs.StartLiveTailEventStream) {
	eventsChan := stream.Events()
	for {
		event := <-eventsChan
		switch e := event.(type) {
		case *types.StartLiveTailResponseStreamMemberSessionStart:
			log.Println("LiveStream start event received")
		case *types.StartLiveTailResponseStreamMemberSessionUpdate:
			for _, logEvent := range e.Value.SessionResults {
				log.Println(*logEvent.Message)
			}
		default:
			if err := stream.Err(); err != nil {
				log.Fatalf("Error occured during streaming: %v", err)
			} else if event == nil {
				log.Println("Stream is Closed")
				return
			} else {
				log.Fatalf("Unknown event type when handling stream: %T", e)
			}
		}
	}
}
