package runtime

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/moby/moby/client"
)

// EnsureImage makes sure an image is available locally on this node.
//
// If the image already exists, nothing is downloaded.
// If it is missing, Build & Break pulls it through Docker.
func (d *DockerRuntime) EnsureImage(
	ctx context.Context,
	image string,
) error {
	if strings.TrimSpace(image) == "" {
		return fmt.Errorf("image name is required")
	}

	exists, err := d.ImageExists(ctx, image)
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	return d.PullImage(ctx, image)
}

// ImageExists checks whether Docker already has the requested
// image available locally.
func (d *DockerRuntime) ImageExists(
	ctx context.Context,
	image string,
) (bool, error) {
	if strings.TrimSpace(image) == "" {
		return false, fmt.Errorf("image name is required")
	}

	checkCtx, cancel := context.WithTimeout(
		ctx,
		10*time.Second,
	)
	defer cancel()

	// The current Moby client uses variadic ImageInspectOption
	// arguments. We don't need any special options here.
	_, err := d.client.ImageInspect(
		checkCtx,
		image,
	)

	if err == nil {
		return true, nil
	}

	// If inspect failed, use ImageList to determine whether the
	// requested reference actually exists locally.
	//
	// This avoids depending on Docker's human-readable error text.
	images, listErr := d.client.ImageList(
		checkCtx,
		client.ImageListOptions{
			All: false,
		},
	)
	if listErr != nil {
		return false, fmt.Errorf(
			"inspect Docker image %q: %w",
			image,
			err,
		)
	}

	for _, localImage := range images.Items {
		for _, tag := range localImage.RepoTags {
			if tag == image {
				return true, nil
			}
		}
	}

	// Docker couldn't inspect it and none of the local image tags
	// match the requested reference, so we consider it missing.
	return false, nil
}

// PullImage downloads an image through the Docker daemon.
func (d *DockerRuntime) PullImage(
	ctx context.Context,
	image string,
) error {
	if strings.TrimSpace(image) == "" {
		return fmt.Errorf("image name is required")
	}

	pullCtx, cancel := context.WithTimeout(
		ctx,
		10*time.Minute,
	)
	defer cancel()

	response, err := d.client.ImagePull(
		pullCtx,
		image,
		client.ImagePullOptions{},
	)
	if err != nil {
		return fmt.Errorf(
			"pull Docker image %q: %w",
			image,
			err,
		)
	}
	defer response.Close()

	// Docker streams the progress of an image pull.
	//
	// We consume the stream completely because reaching the end
	// tells us Docker has finished processing the pull.
	//
	// Later we can expose this progress through provisioning jobs.
	if _, err := io.Copy(io.Discard, response); err != nil {
		return fmt.Errorf(
			"read Docker image pull response: %w",
			err,
		)
	}

	return nil
}
