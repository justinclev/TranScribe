// Package cdk generates AWS CDK TypeScript projects from a Blueprint. AWS-only.
//
// Output files:
//
//	cdk.json      — CDK toolkit configuration
//	package.json  — npm dependencies
//	bin/app.ts    — CDK app entry point
//	lib/stack.ts  — CDK Stack defining VPC, IAM roles, ECS Fargate services
package cdk

import (
	"strings"
	"text/template"

	"github.com/justinclev/transcribe/internal/generator/render"
	"github.com/justinclev/transcribe/internal/models"
)

// Generate writes all CDK TypeScript files into outputDir.
func Generate(bp *models.Blueprint, outputDir string) error {
	return render.WriteFiles(outputDir, []struct{ Name, Tmpl string }{
		{"cdk.json", configTmpl},
		{"package.json", packageTmpl},
		{"bin/app.ts", appTmpl},
		{"lib/stack.ts", stackTmpl},
	}, bp, cdkFuncMap())
}

func cdkFuncMap() template.FuncMap {
	return template.FuncMap{
		"firstPort": func(ports []string) string {
			if len(ports) == 0 {
				return "80"
			}
			p := ports[0]
			if idx := strings.Index(p, ":"); idx >= 0 {
				return p[idx+1:]
			}
			return p
		},
	}
}

const configTmpl = `{
  "app": "npx ts-node bin/app.ts",
  "context": {
    "Transcribe": "true"
  }
}
`

const packageTmpl = `{
  "name": "{{.Name}}",
  "scripts": {
    "build": "tsc",
    "synth": "cdk synth",
    "deploy": "cdk deploy"
  },
  "devDependencies": {
    "@types/node":         "^18",
    "typescript":          "^5",
    "ts-node":             "^10",
    "aws-cdk":             "^2"
  },
  "dependencies": {
    "aws-cdk-lib":         "^2",
    "constructs":          "^10"
  }
}
`

const appTmpl = `#!/usr/bin/env node
import "source-map-support/register";
import * as cdk from "aws-cdk-lib";
import { {{tfid .Name}}Stack } from "../lib/stack";

const app = new cdk.App();
new {{tfid .Name}}Stack(app, "{{.Name}}", {
    env: { region: "{{.Region}}" },
    tags: { Transcribe: "true" },
});
`

const stackTmpl = `import * as cdk  from "aws-cdk-lib";
import * as ec2  from "aws-cdk-lib/aws-ec2";
import * as ecs  from "aws-cdk-lib/aws-ecs";
import * as iam  from "aws-cdk-lib/aws-iam";
import { Construct } from "constructs";

export class {{tfid .Name}}Stack extends cdk.Stack {
    constructor(scope: Construct, id: string, props?: cdk.StackProps) {
        super(scope, id, props);

        // ── VPC ───────────────────────────────────────────────────────────────

        const vpc = new ec2.Vpc(this, "{{.Name}}-vpc", {
            ipAddresses:        ec2.IpAddresses.cidr("{{.Network.VPCCidr}}"),
            maxAzs:             2,
            natGateways:        1,
            subnetConfiguration: [
                {
                    name:       "public",
                    subnetType: ec2.SubnetType.PUBLIC,
                    cidrMask:   24,
                },
                {
                    name:       "private",
                    subnetType: ec2.SubnetType.PRIVATE_WITH_EGRESS,
                    cidrMask:   24,
                },
            ],
        });

        cdk.Tags.of(vpc).add("Name", "{{.Name}}-vpc");

        // ── IAM task roles ────────────────────────────────────────────────────
{{range .Services}}
        const role_{{tfid .IAMRoleName}} = new iam.Role(this, "{{.IAMRoleName}}", {
            roleName:  "{{.IAMRoleName}}",
            assumedBy: new iam.ServicePrincipal("ecs-tasks.amazonaws.com"),
        });

        role_{{tfid .IAMRoleName}}.addToPolicy(new iam.PolicyStatement({
            sid:       "CloudWatchLogs",
            effect:    iam.Effect.ALLOW,
            actions:   ["logs:CreateLogStream", "logs:PutLogEvents"],
            resources: ["*"],
        }));

        role_{{tfid .IAMRoleName}}.addToPolicy(new iam.PolicyStatement({
            sid:       "ECRPull",
            effect:    iam.Effect.ALLOW,
            actions:   [
                "ecr:GetAuthorizationToken",
                "ecr:BatchCheckLayerAvailability",
                "ecr:GetDownloadUrlForLayer",
                "ecr:BatchGetImage",
            ],
            resources: ["*"],
        }));

        cdk.Tags.of(role_{{tfid .IAMRoleName}}).add("Name", "{{.IAMRoleName}}");
        cdk.Tags.of(role_{{tfid .IAMRoleName}}).add("Transcribe", "true");
{{end}}
        // ── ECS Cluster ───────────────────────────────────────────────────────

        const cluster = new ecs.Cluster(this, "{{.Name}}-cluster", {
            vpc,
            clusterName: "{{.Name}}-cluster",
        });

        cdk.Tags.of(cluster).add("Transcribe", "true");

        // ── Fargate task definitions & services ───────────────────────────────
{{range .Services}}
        const taskDef_{{tfid .Name}} = new ecs.FargateTaskDefinition(this, "{{.Name}}-task", {
            family:        "{{.Name}}",
            cpu:           {{.CPU}},
            memoryLimitMiB: {{.Memory}},
            taskRole:      role_{{tfid .IAMRoleName}},
        });

        taskDef_{{tfid .Name}}.addContainer("{{.Name}}", {
            image:         ecs.ContainerImage.fromRegistry("{{.Image}}"),
            portMappings:  [{ containerPort: {{firstPort .Ports}} }],
            logging:       ecs.LogDrivers.awsLogs({ streamPrefix: "{{.Name}}" }),
        });

        new ecs.FargateService(this, "{{.Name}}-svc", {
            cluster,
            taskDefinition: taskDef_{{tfid .Name}},
            desiredCount:   {{.MinCount}},
            assignPublicIp: false,
            vpcSubnets:    { subnetType: ec2.SubnetType.PRIVATE_WITH_EGRESS },
        });
{{end}}
    }
}
`
