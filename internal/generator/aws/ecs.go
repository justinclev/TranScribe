// Package aws — ecs.tf template.
package aws

// ecsTmpl renders ecs.tf.
// Creates: ECS cluster (Container Insights), task execution role,
// per-service task definitions (Fargate/awsvpc) and ECS services.
const ecsTmpl = `# ── ECS Cluster ──────────────────────────────────────────────────────────────

resource "aws_ecs_cluster" "{{tfid .Name}}" {
  name = "{{.Name}}"

  setting {
    name  = "containerInsights"
    value = "enabled"
  }

  tags = {
    Name = "{{.Name}}-cluster"
  }
}

resource "aws_ecs_cluster_capacity_providers" "{{tfid .Name}}" {
  cluster_name       = aws_ecs_cluster.{{tfid .Name}}.name
  capacity_providers = ["FARGATE", "FARGATE_SPOT"]

  default_capacity_provider_strategy {
    base              = 1
    weight            = 100
    capacity_provider = "FARGATE"
  }
}

# ── ECS Task Execution Role ───────────────────────────────────────────────────
# Separate from the per-service task role: this role is used by the ECS agent
# to pull images from ECR and push logs to CloudWatch.

resource "aws_iam_role" "{{tfid .Name}}_execution" {
  name = "{{.Name}}-ecs-execution-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "ecs-tasks.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })

  tags = {
    Name = "{{.Name}}-ecs-execution-role"
  }
}

resource "aws_iam_role_policy_attachment" "{{tfid .Name}}_execution" {
  role       = aws_iam_role.{{tfid .Name}}_execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

# Allow the execution role to read secrets from Secrets Manager
resource "aws_iam_role_policy" "{{tfid .Name}}_execution_secrets" {
  name = "{{.Name}}-execution-secrets"
  role = aws_iam_role.{{tfid .Name}}_execution.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Sid    = "SecretsManager"
      Effect = "Allow"
      Action = [
        "secretsmanager:GetSecretValue",
        "kms:Decrypt",
      ]
      Resource = "*"
    }]
  })
}
{{range .Services}}
# ── ECS Task Definition: {{.Name}} ────────────────────────────────────────────

resource "aws_ecs_task_definition" "{{tfid .Name}}" {
  family                   = "{{$.Name}}-{{.Name}}"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = {{.CPU}}
  memory                   = {{.Memory}}
  execution_role_arn       = aws_iam_role.{{tfid $.Name}}_execution.arn
  task_role_arn            = aws_iam_role.{{tfid .IAMRoleName}}.arn

  container_definitions = jsonencode([{
    name      = "{{.Name}}"
    image     = "{{if .Image}}{{.Image}}{{else}}${aws_ecr_repository.{{tfid .Name}}.repository_url}:latest{{end}}"
    essential = true
{{- if .Ports}}
    portMappings = [{{range .Ports}}
      {
        containerPort = {{firstPort (list .)}}
        hostPort      = {{firstPort (list .)}}
        protocol      = "tcp"
      },{{end}}
    ]
{{- end}}
    logConfiguration = {
      logDriver = "awslogs"
      options = {
        "awslogs-group"         = aws_cloudwatch_log_group.{{tfid .Name}}.name
        "awslogs-region"        = "{{$.Region}}"
        "awslogs-stream-prefix" = "ecs"
      }
    }
{{- if .EnvVars}}
    secrets = [{{range $k, $v := .EnvVars}}
      {
        name      = "{{$k}}"
        valueFrom = aws_secretsmanager_secret.{{tfid $.Name}}_{{tfid $k}}.arn
      },{{end}}
    ]
{{- end}}
  }])

  tags = {
    Name = "{{$.Name}}-{{.Name}}"
  }
}

resource "aws_ecs_service" "{{tfid .Name}}" {
  name            = "{{$.Name}}-{{.Name}}"
  cluster         = aws_ecs_cluster.{{tfid $.Name}}.id
  task_definition = aws_ecs_task_definition.{{tfid .Name}}.arn
  desired_count   = {{.MinCount}}
  launch_type     = "FARGATE"

  network_configuration {
    subnets          = [
      aws_subnet.{{tfid $.Name}}_private_1.id,
      aws_subnet.{{tfid $.Name}}_private_2.id,
    ]
    security_groups  = [aws_security_group.{{tfid $.Name}}_ecs.id]
    assign_public_ip = false
  }
{{- if .Ports}}

  load_balancer {
    target_group_arn = aws_lb_target_group.{{tfid .Name}}.arn
    container_name   = "{{.Name}}"
    container_port   = {{firstPort .Ports}}
  }
{{- end}}

  deployment_minimum_healthy_percent = 100
  deployment_maximum_percent         = 200

  deployment_circuit_breaker {
    enable   = true
    rollback = true
  }

  enable_execute_command = false

  tags = {
    Name = "{{$.Name}}-{{.Name}}"
  }

  depends_on = [aws_iam_role_policy_attachment.{{tfid $.Name}}_execution]
}
{{- if gt .MaxCount .MinCount}}
# ── Application Auto Scaling: {{.Name}} ──────────────────────────────────────

resource "aws_appautoscaling_target" "{{tfid .Name}}" {
  max_capacity       = {{.MaxCount}}
  min_capacity       = {{.MinCount}}
  resource_id        = "service/${aws_ecs_cluster.{{tfid $.Name}}.name}/${aws_ecs_service.{{tfid .Name}}.name}"
  scalable_dimension = "ecs:service:DesiredCount"
  service_namespace  = "ecs"
}

resource "aws_appautoscaling_policy" "{{tfid .Name}}_cpu" {
  name               = "{{$.Name}}-{{.Name}}-cpu-scaling"
  policy_type        = "TargetTrackingScaling"
  resource_id        = aws_appautoscaling_target.{{tfid .Name}}.resource_id
  scalable_dimension = aws_appautoscaling_target.{{tfid .Name}}.scalable_dimension
  service_namespace  = aws_appautoscaling_target.{{tfid .Name}}.service_namespace

  target_tracking_scaling_policy_configuration {
    predefined_metric_specification {
      predefined_metric_type = "ECSServiceAverageCPUUtilization"
    }
    target_value       = 70.0
    scale_in_cooldown  = 300
    scale_out_cooldown = 60
  }
}

resource "aws_appautoscaling_policy" "{{tfid .Name}}_memory" {
  name               = "{{$.Name}}-{{.Name}}-memory-scaling"
  policy_type        = "TargetTrackingScaling"
  resource_id        = aws_appautoscaling_target.{{tfid .Name}}.resource_id
  scalable_dimension = aws_appautoscaling_target.{{tfid .Name}}.scalable_dimension
  service_namespace  = aws_appautoscaling_target.{{tfid .Name}}.service_namespace

  target_tracking_scaling_policy_configuration {
    predefined_metric_specification {
      predefined_metric_type = "ECSServiceAverageMemoryUtilization"
    }
    target_value       = 80.0
    scale_in_cooldown  = 300
    scale_out_cooldown = 60
  }
}
{{- end}}
{{end}}`
