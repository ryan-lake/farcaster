package main

import (
	"flag"
	"fmt"
	"log"
	"sync"
	"time"
)

type EbFunction struct {
	EventName    string
	BusName      string
	FunctionName string
	FunctionArn  string
	LogGroup     string
}

func main() {
	targetEvent := flag.String("event", "", "Name of the event to start tailing")
	showEvents := flag.Bool("v", false, "Verbose")
	flag.Parse()
	c_lambInfo := make(chan []LambdaInfo)
	c_eventInfo := make(chan []EbRule)
	var wg_lambda sync.WaitGroup
	var wg_event sync.WaitGroup

	wg_lambda.Add(1)
	go FetchLambdaInfoAsync(&wg_lambda, c_lambInfo)
	wg_event.Add(1)
	go GetEventbridgeRulesAsync(&wg_event, c_eventInfo)

	go func() {
		wg_lambda.Wait()
		wg_event.Wait()
		close(c_eventInfo)
		close(c_lambInfo)
	}()

	lambdas := <-c_lambInfo
	rules := <-c_eventInfo

	mappedEvents := MapLambdaToEvents(lambdas, rules)
	if *showEvents {
		for i, me := range mappedEvents {
			fmt.Printf("%v: Event Name: %v, Bus: %v, Function: %v, LogGroup:%v\n", i, me.EventName, me.BusName, me.FunctionName, me.LogGroup)
		}
		log.Printf("Found %v Events in current session \n", len(rules))
		log.Printf("Found %v functions in current session \n", len(lambdas))
	}
	if *targetEvent == "" {
		log.Fatal("No event provided, exiting...")
	}

	log.Println("Beginning LiveLog...")
	var group string
	for _, me := range mappedEvents {
		if me.EventName == *targetEvent {
			group = me.LogGroup
			break
		}
	}
	if group == "" {
		log.Fatalln("Failed to find loggroup for event.")
	}
	BeginLiveStream(group)
	for {
		time.Sleep(5000)
	}
}

func MapLambdaToEvents(functions []LambdaInfo, events []EbRule) []EbFunction {
	var mappedEvents []EbFunction
	for _, e := range events {
		for _, f := range functions {
			if e.FunctionArn == f.Arn {
				mappedEvents = append(mappedEvents, EbFunction{EventName: e.DetailType, BusName: e.BusName, FunctionName: f.Name, FunctionArn: f.Arn, LogGroup: f.LogGroup})
			}
		}
	}
	return mappedEvents
}
