// Package cdk generates AWS CDK TypeScript projects from a Blueprint. AWS-only.
//
// Output files:
//
//	cdk.json      — CDK toolkit configuration
//	package.json  — npm dependencies
//	bin/app.ts    — CDK app entry point
//	lib/stack.ts  — CDK Stack defining VPC, IAM roles
package cdk

import (
	"github.com/justinclev/transcribe/internal/generator/render"
	"github.com/justinclev/transcribe/pkg/models"
)

// Generate writes all CDK TypeScript files into outputDir.
func Generate(bp *models.Blueprint, outputDir string) error {
	return render.WriteFiles(outputDir, []struct{ Name, Tmpl string }{
		{"cdk.json", configTmpl},
		{"package.json", packageTmpl},
		{"bin/app.ts", appTmpl},
		{"lib/stack.ts", stackTmpl},
	}, bp, nil)
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
    }
}
`
