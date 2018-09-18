<img src="https://media.dev.unee-t.com/2018-09-11/estimate.png" alt="Summarise AWS expenditure">

Requires billing alerts to be enabled in your billing preferences & CloudWatchReadOnlyAccess in the role
https://docs.aws.amazon.com/awsaccountbilling/latest/aboutv2/monitor-charges.html

Reference case #5314191201

# Updates twice a day

Triggered at the start & end of the working day in Singapore

<img src=https://s.natalian.org/2018-09-11/1536675100_1534x1406.png>

# Deployment notes

Currently the code is defined for my use case and accounts. Notice two accounts
are in an organisation and another uses a cross account role to get the
metrics.

There are many ways to deploy a serverless function, however I'm using
http://apex.run/ in this instance. The `project.json` looks like:

	{
	  "name": "estimatedcharges",
	  "description": "Post to slack a summary of the estimated charges of the AWS account",
	  "profile": "my-profile",
	  "memory": 128,
	  "timeout": 5,
	  "role": "arn:aws:iam::812644853088:role/estimatedcharges_lambda_function",
	  "environment": {
		"WEBHOOK": "https://hooks.slack.com/services/XXXXX/YYYYYY/etcetc"
	  }
	}
