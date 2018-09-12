package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"time"

	"github.com/apex/log"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/endpoints"
	"github.com/aws/aws-sdk-go-v2/aws/external"
	"github.com/aws/aws-sdk-go-v2/aws/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

type attachment struct {
	Fallback  string `json:"fallback"`
	Color     string `json:"color"`
	Title     string `json:"title"`
	TitleLink string `json:"title_link,omitempty"`
	Text      string `json:"text,omitempty"`
	Ts        int64  `json:"ts"`
}

func main() {
	lambda.Start(handler)

}

func handler(ctx context.Context, snsEvent events.SNSEvent) {

	cfg, err := external.LoadDefaultAWSConfig(external.WithSharedConfigProfile("mine"))
	if err != nil {
		log.WithError(err).Fatal("setting up credentials")
		return
	}
	// Can only get billing info from us-east-1
	cfg.Region = endpoints.UsEast1RegionID
	demoresp, err := estimatedCharges(cfg, "915001051872")
	if err != nil {
		log.WithError(err).Fatal("unable to retrieve bill estimate for demo account")
	}

	devresp, err := estimatedCharges(cfg, "812644853088")
	if err != nil {
		log.WithError(err).Fatal("unable to retrieve bill estimate for dev account")
	}

	cfg.Credentials = stscreds.NewAssumeRoleProvider(sts.New(cfg), "arn:aws:iam::192458993663:role/estimatedcharges")
	prodresp, err := estimatedCharges(cfg, "")
	if err != nil {
		log.WithError(err).Fatal("unable to retrieve bill estimate for dev account")
	}

	var attachments []attachment
	attachments = append(attachments, slackTemplate(demoresp, "Demo 915001051872")...)
	attachments = append(attachments, slackTemplate(devresp, "Dev 812644853088")...)
	attachments = append(attachments, slackTemplate(prodresp, "Prod 192458993663")...)

	type slackPayload struct {
		Attachments []attachment `json:"attachments"`
	}

	jsonValue, _ := json.Marshal(slackPayload{Attachments: attachments})

	presp, err := http.Post(os.Getenv("WEBHOOK"), "application/json", bytes.NewBuffer(jsonValue))
	if err != nil {
		panic(err)
	}
	if presp.StatusCode != http.StatusOK {
		log.Fatalf("Post failed: %+v", presp)
	}
}

func estimatedCharges(cfg aws.Config, linkedAccount string) (resp *cloudwatch.GetMetricStatisticsOutput, err error) {

	now := time.Now()
	before := now.Add(-time.Hour * 24 * 2)

	svc := cloudwatch.New(cfg)
	// https://godoc.org/github.com/aws/aws-sdk-go-v2/service/cloudwatch#GetMetricStatisticsRequest
	// https://godoc.org/github.com/aws/aws-sdk-go-v2/service/cloudwatch#GetMetricStatisticsInput
	req := svc.GetMetricStatisticsRequest(&cloudwatch.GetMetricStatisticsInput{
		Dimensions: []cloudwatch.Dimension{
			{
				Name:  aws.String("Currency"),
				Value: aws.String("USD"),
			},
		},
		EndTime:    &now,
		MetricName: aws.String("EstimatedCharges"),
		Namespace:  aws.String("AWS/Billing"),
		Period:     aws.Int64(int64(28800)), // 8hrs periods
		StartTime:  &before,
		Statistics: []cloudwatch.Statistic{cloudwatch.StatisticMaximum},
	})

	if linkedAccount != "" {
		req.Input.Dimensions = append(req.Input.Dimensions, cloudwatch.Dimension{
			Name:  aws.String("LinkedAccount"),
			Value: aws.String(linkedAccount),
		})
	}

	resp, err = req.Send()
	return
}

func slackTemplate(resp *cloudwatch.GetMetricStatisticsOutput, profile string) (attachments []attachment) {

	// fmt.Printf("%+v\n", resp.Datapoints)
	sort.Slice(resp.Datapoints, func(i, j int) bool {
		time1 := *(resp.Datapoints[i].Timestamp)
		time2 := *(resp.Datapoints[j].Timestamp)
		return time1.Before(time2)
	})

	var preincrease float64
	var highestDerivative float64
	var lastTime int64
	var text string
	for i := 0; i < len(resp.Datapoints); i++ {
		if i < len(resp.Datapoints)-1 {
			now := *(resp.Datapoints[i].Maximum)
			next := *(resp.Datapoints[i+1].Maximum)
			timestamp := *(resp.Datapoints[i+1].Timestamp)
			increase := next - now

			// Duplicate value is just noise, so skip
			if increase == 0 {
				continue
			}

			derivative := increase - preincrease
			log.Infof("%s, Before: %.2f, Now: %.2f, Increase: %.2f, Derivative: %.2f", time.Since(timestamp), now, next, increase, derivative)

			if preincrease != 0 {
				text += fmt.Sprintf("• %dh ago was: %.1f, 8hrs later: %.1f, Increase: *%.2f*, Derivative: %.2f\n",
					int(time.Since(timestamp).Hours()), now, next, increase, derivative)
				if highestDerivative < derivative {
					highestDerivative = derivative
				}
			}

			// For next loop
			preincrease = increase
			lastTime = timestamp.Unix()
		}
	}

	// https://api.slack.com/docs/message-attachments
	// good, warning, danger
	color := "good"
	if highestDerivative > 1 {
		color = "danger"
	} else if highestDerivative > 0.5 {
		color = "warning"
	}

	return append(attachments, attachment{
		Fallback: text,
		Color:    color,
		Title:    fmt.Sprintf("%.2f on %s", highestDerivative, profile),
		Text:     text,
		Ts:       lastTime,
	})
}
