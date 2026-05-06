// Package ecr authenticates the local Docker daemon to an Amazon ECR
// registry. It calls ECR's GetAuthorizationToken (using the default AWS
// credential chain so the same code path serves laptop and CI), decodes the
// returned token, and invokes `docker login --password-stdin`.
package ecr

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
)

// TokenGetter is the slice of the AWS SDK we depend on, lifted into an
// interface so tests can substitute a stub.
type TokenGetter interface {
	GetAuthorizationToken(ctx context.Context, in *ecr.GetAuthorizationTokenInput, opts ...func(*ecr.Options)) (*ecr.GetAuthorizationTokenOutput, error)
}

// DockerLoginFunc runs `docker login` for a registry. The password is
// supplied via stdin so it never appears in process arguments or logs.
type DockerLoginFunc func(ctx context.Context, registry, username, password string) error

// Authenticator owns the ECR client and the docker-login function. Both
// fields are exported so tests can swap them.
type Authenticator struct {
	Client TokenGetter
	Login  DockerLoginFunc
}

// New constructs an Authenticator using the default AWS credential chain
// (env, shared config, SSO, IMDS), optionally pinned to a region.
func New(ctx context.Context, region string) (*Authenticator, error) {
	var opts []func(*awsconfig.LoadOptions) error
	if region != "" {
		opts = append(opts, awsconfig.WithRegion(region))
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}
	return &Authenticator{
		Client: ecr.NewFromConfig(cfg),
		Login:  realDockerLogin,
	}, nil
}

// LoginToECR fetches an ECR authorization token, decodes it into a username
// and password, and runs docker login on the registry endpoint the token
// authorizes. It returns the registry hostname so callers can construct
// fully-qualified image references.
func (a *Authenticator) LoginToECR(ctx context.Context) (registry string, err error) {
	out, err := a.Client.GetAuthorizationToken(ctx, &ecr.GetAuthorizationTokenInput{})
	if err != nil {
		return "", fmt.Errorf("ECR GetAuthorizationToken: %w", err)
	}
	if len(out.AuthorizationData) == 0 {
		return "", errors.New("ECR GetAuthorizationToken returned no authorization data")
	}
	data := out.AuthorizationData[0]
	if data.AuthorizationToken == nil || data.ProxyEndpoint == nil {
		return "", errors.New("ECR authorization data is missing token or endpoint")
	}
	raw, err := base64.StdEncoding.DecodeString(aws.ToString(data.AuthorizationToken))
	if err != nil {
		return "", fmt.Errorf("decode ECR token: %w", err)
	}
	user, pass, ok := strings.Cut(string(raw), ":")
	if !ok || user == "" || pass == "" {
		return "", errors.New("ECR token is not in user:password form")
	}
	registry = aws.ToString(data.ProxyEndpoint)
	if err := a.Login(ctx, registry, user, pass); err != nil {
		// Wrap deliberately omits the password.
		return registry, fmt.Errorf("docker login %s: %w", registry, err)
	}
	return registry, nil
}

// realDockerLogin runs `docker login --username <user> --password-stdin
// <registry>` with the password piped on stdin.
func realDockerLogin(ctx context.Context, registry, username, password string) error {
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("docker CLI not on PATH (install Docker or run from CI): %w", err)
	}
	host := strings.TrimPrefix(strings.TrimPrefix(registry, "https://"), "http://")
	cmd := exec.CommandContext(ctx, "docker", "login", "--username", username, "--password-stdin", host)
	cmd.Stdin = strings.NewReader(password)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Output may include "Login Succeeded" or an error. Trim and surface
		// the tail; password is not in the args, only on stdin.
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
