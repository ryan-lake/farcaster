package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge/types"
)

type EbRule struct {
	DetailType  string
	BusName     string
	FunctionArn string
}

type TargetResult struct {
	BusName  string
	RuleName string
	Targets  []types.Target
}

func GetEventbridgeRulesAsync(wg *sync.WaitGroup, channel chan []EbRule) {
	rules, err := GetEventbridgeRules()
	if err != nil {
		log.Println("Error getting event bridge info")
		wg.Done()
		return
	}
	channel <- rules
}

func GetEventbridgeRules() ([]EbRule, error) {
	config, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion("us-east-2"))
	if err != nil {
		log.Println("Error building aws config")
	}
	log.Println("Begin fetching Rules")
	svc := eventbridge.NewFromConfig(config)
	allRules := []types.Rule{}
	busses := []string{"control-event-bus", "resource-event-bus"}
	for _, busName := range busses {
		rules, err := GetRules(svc, busName)
		if err != nil {
			log.Println("Failed getting rules for bus:", busName)
		}
		allRules = append(allRules, rules...)
	}
	var wg sync.WaitGroup
	resultsChan := make(chan EbRule)
	log.Println("Rules fetched, fetch targets.")
	for _, r := range allRules {
		wg.Add(1)
		go GetTargetsForRuleAsync(svc, r, &wg, resultsChan)
	}

	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	var rules []EbRule
	for {
		result, ok := <-resultsChan
		if !ok {
			break
		}
		rules = append(rules, result)
	}
	log.Println("Rules and Targets built")
	return rules, nil
}

func GetRules(svc *eventbridge.Client, busName string) ([]types.Rule, error) {
	rules := []types.Rule{}
	for {
		result, err := svc.ListRules(context.TODO(), &eventbridge.ListRulesInput{EventBusName: &busName})
		if err != nil {
			log.Println("Failed getting bus rules for bus:", busName)
			return nil, err
		}
		rules = append(rules, result.Rules...)
		if result.NextToken == nil {
			break
		}
	}
	return rules, nil
}

func GetTargetsForRuleAsync(svc *eventbridge.Client, rule types.Rule, wg *sync.WaitGroup, resultChan chan EbRule) {
	targets, err := svc.ListTargetsByRule(context.Background(), &eventbridge.ListTargetsByRuleInput{Rule: rule.Name, EventBusName: rule.EventBusName})
	if err != nil {
		log.Println("Error fetching targets for rule: ", *rule.Name, "on bus:", *rule.EventBusName)
		return
	}

	result, err := BuildEbRule(*rule.EventBusName, *rule.EventPattern, targets.Targets[0])
	if err != nil {
		wg.Done()
		return
	}
	resultChan <- result
	wg.Done()
}

func BuildEbRule(busName string, pattern string, target types.Target) (EbRule, error) {

	type Pattern struct {
		DetailType []string `json:"detail-type"`
		Source     []string `json:"source"`
	}
	var pat Pattern
	err := json.Unmarshal([]byte(pattern), &pat)
	if err != nil {
		fmt.Println(err)
	}
	if len(pat.DetailType) == 0 {
		return EbRule{}, errors.New("no detailtype for rule")
	}
	return EbRule{BusName: busName, DetailType: pat.DetailType[0], FunctionArn: *target.Arn}, nil
}
