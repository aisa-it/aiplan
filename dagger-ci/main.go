// A generated module for Aiplan functions
//
// This module has been generated via dagger init and serves as a reference to
// basic module structure as you get started with Dagger.
//
// Two functions have been pre-created. You can modify, delete, or add to them,
// as needed. They demonstrate usage of arguments and return types using simple
// echo and grep commands. The functions can be called from the dagger CLI or
// from one of the SDKs.
//
// The first line in this comment block is a short description line and the
// rest is a long description with more detail on the module's purpose or usage,
// if appropriate. All modules should have a short description.

package main

import (
	"context"
	"dagger/aiplan/internal/dagger"
	"fmt"
	"strings"
)

type Aiplan struct{}

func (m *Aiplan) GoBuildEnv(source *dagger.Directory) *dagger.Container {
	goCache := dag.CacheVolume("go")
	return dag.Container().
		From("golang:alpine").
		WithDirectory("/src", source.Directory("aiplan.go/")).
		WithWorkdir("/src").
		WithEnvVariable("GOOS", "linux").
		WithMountedCache("/go/pkg/mod", goCache).
		WithExec([]string{"apk", "add", "curl"}).
		WithExec([]string{"go", "mod", "tidy"})
}

func (m *Aiplan) FrontBuildEnv(version string, source *dagger.Directory) *dagger.Container {
	nodeCache := dag.CacheVolume("node")
	quasarCache := dag.CacheVolume("next")

	return dag.Container().
		From("node:20.8.0").
		WithDirectory("/src", source).
		WithWorkdir("/src").
		WithMountedCache("/src/node_modules", nodeCache).
		WithMountedCache("/src/.quasar", quasarCache).
		WithExec([]string{"yarn"}).
		WithExec([]string{"yarn", "version", "--new-version", strings.TrimLeft(version, "v"), "--no-git-tag-version"}).
		WithExec([]string{"yarn", "build"})
}

func (m *Aiplan) BackEnv(platform dagger.Platform, appBin *dagger.File, schema *dagger.File, docs *dagger.Directory, spa *dagger.Directory) *dagger.Container {
	return dag.Container(dagger.ContainerOpts{
		Platform: platform,
	}).
		From("alpine").
		WithEnvVariable("TZ", "Europe/Moscow").
		WithExec([]string{"apk", "add", "curl"}).
		WithExec([]string{"apk", "add", "--no-cache", "tzdata"}).
		WithWorkdir("/app").
		WithFile("/app/app", appBin).
		WithDirectory("/app/aiplan-help", docs).
		WithDirectory("/app/spa", spa).
		WithEnvVariable("FRONT_PATH", "/app/spa").
		WithEntrypoint([]string{"/app/app"})
}

func (m *Aiplan) Build(version string, source *dagger.Directory) []*dagger.Container {
	buildMatrix := []struct {
		Arch     string
		BinName  string
		Platform dagger.Platform
	}{
		{
			Arch:     "amd64",
			BinName:  "/build/aiplan-linux",
			Platform: dagger.Platform("linux/amd64"),
		},
		{
			Arch:     "arm64",
			BinName:  "/build/aiplan-linux-arm64",
			Platform: dagger.Platform("linux/arm64/v8"),
		},
	}

	var images []*dagger.Container
	for _, buildParam := range buildMatrix {
		builder := m.GoBuildEnv(source).
			WithEnvVariable("GOARCH", buildParam.Arch).
			WithExec([]string{"go", "build", "-o", buildParam.BinName, "-ldflags", fmt.Sprintf("-s -w -X main.version=%s", version), "cmd/aiplan/main.go"})

		front := m.FrontBuildEnv(version, source.Directory("aiplan-front/"))

		image := m.BackEnv(
			buildParam.Platform,
			builder.File(buildParam.BinName),
			builder.File("/src/schema.sql"),
			source.Directory("aiplan-help/"),
			front.Directory("/src/dist/pwa"),
		).
			WithLabel("org.opencontainers.image.source", "https://github.com/aisa-it/aiplan").
			WithLabel("org.opencontainers.image.licenses", "MPL-2.0").
			WithAnnotation("org.opencontainers.image.source", "https://github.com/aisa-it/aiplan")
		images = append(images, image)
	}
	return images
}

func (m *Aiplan) BuildPromo(ctx context.Context, version string, source *dagger.Directory, demoSecret *dagger.Secret) (string, error) {
	image := dag.Container(dagger.ContainerOpts{Platform: dagger.Platform("linux/amd64")}).
		From("caddy:alpine").
		WithFile("/etc/caddy/Caddyfile", source.File("Caddyfile")).
		WithDirectory("/srv", source.Directory("src/")).
		WithRegistryAuth("registry.aiplan.aisa.ru", "aiplan", demoSecret)

	return image.Publish(ctx, fmt.Sprintf("registry.aiplan.aisa.ru/aiplan:promo-%s", version))
}

func (m *Aiplan) Helm(
	ctx context.Context,
	kubeDir *dagger.Directory,
	chartName string,
	version string,
	ciApiUrl string,
	ciProjectId string,
	channel string,
	gitlabToken string,
	demoToken string,
	demoChartUrl string,
) (string, error) {
	return dag.Container().
		From("alpine").
		WithExec([]string{"apk", "add", "curl", "helm"}).
		WithDirectory("/src/", kubeDir).
		WithWorkdir("/src").
		WithEntrypoint([]string{"sh", "-c"}).
		WithExec(strings.Split(fmt.Sprintf("helm package %s --version %s --app-version %s", chartName, version, version), " ")).
		// Upload to gitlab
		WithExec([]string{"sh", "-c", fmt.Sprintf("curl --request POST --user gitlab-ci-token:%s --form \"chart=@/src/%s-%s.tgz\" \"%s/projects/%s/packages/helm/api/%s/charts\"",
			gitlabToken,
			chartName,
			version,
			ciApiUrl,
			ciProjectId,
			channel,
		)}).
		// Upload to demo
		WithExec([]string{"sh", "-c", fmt.Sprintf("curl --request POST --user aiplan:%s --data-binary \"@/src/%s-%s.tgz\" \"%s/api/charts\"",
			demoToken,
			chartName,
			version,
			demoChartUrl,
		)}).
		Stdout(ctx)
}

func (m *Aiplan) Publish(
	ctx context.Context,
	images []*dagger.Container,
	registrySecret *dagger.Secret,
	registryUser string,
	imageName string,
) (string, error) {

	registry := dag.Container().
		WithRegistryAuth("ghcr.io", registryUser, registrySecret)

	refGitlab, err := registry.
		Publish(ctx, "ghcr.io/"+imageName, dagger.ContainerPublishOpts{PlatformVariants: images})
	if err != nil {
		return refGitlab, err
	}

	return refGitlab, err
}

func (m *Aiplan) Export(
	ctx context.Context,
	images []*dagger.Container,
	imageName string,
) (string, error) {
	return dag.Container().
		Export(ctx, imageName, dagger.ContainerExportOpts{PlatformVariants: images})
}

func (m *Aiplan) BuildLocal(ctx context.Context, name string, source *dagger.Directory) (string, error) {
	return m.Export(ctx, m.Build("v0.1.0", source), name)
}

func (m *Aiplan) BuildApp(ctx context.Context, version string, source *dagger.Directory,
	registrySecret *dagger.Secret,
	registryUser string,
	imageName string,
) error {
	back := m.Build(version, source)

	backRef, err := m.Publish(ctx, back, registrySecret, registryUser, fmt.Sprintf("%s:%s", imageName, version))
	if err != nil {
		return err
	}
	fmt.Println(backRef)

	backRef, err = m.Publish(ctx, back, registrySecret, registryUser, fmt.Sprintf("%s:%s", imageName, "latest"))
	if err != nil {
		return err
	}
	fmt.Println(backRef)

	return nil
}
