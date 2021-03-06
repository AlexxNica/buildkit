package containerimage

import (
	"time"

	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/images"
	"github.com/moby/buildkit/cache"
	"github.com/moby/buildkit/exporter"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/util/push"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
)

const (
	keyImageName        = "name"
	keyPush             = "push"
	keyInsecure         = "registry.insecure"
	exporterImageConfig = "containerimage.config"
)

type Opt struct {
	SessionManager *session.Manager
	ImageWriter    *ImageWriter
	Images         images.Store
}

type imageExporter struct {
	opt Opt
}

func New(opt Opt) (exporter.Exporter, error) {
	im := &imageExporter{opt: opt}
	return im, nil
}

func (e *imageExporter) Resolve(ctx context.Context, opt map[string]string) (exporter.ExporterInstance, error) {
	i := &imageExporterInstance{imageExporter: e}
	for k, v := range opt {
		switch k {
		case keyImageName:
			i.targetName = v
		case keyPush:
			i.push = true
		case keyInsecure:
			i.insecure = true
		case exporterImageConfig:
			i.config = []byte(v)
		default:
			logrus.Warnf("image exporter: unknown option %s", k)
		}
	}
	return i, nil
}

type imageExporterInstance struct {
	*imageExporter
	targetName string
	push       bool
	insecure   bool
	config     []byte
}

func (e *imageExporterInstance) Name() string {
	return "exporting to image"
}

func (e *imageExporterInstance) Export(ctx context.Context, ref cache.ImmutableRef, opt map[string][]byte) error {
	if config, ok := opt[exporterImageConfig]; ok {
		e.config = config
	}
	desc, err := e.opt.ImageWriter.Commit(ctx, ref, e.config)
	if err != nil {
		return err
	}

	if e.targetName != "" {
		if e.opt.Images != nil {
			tagDone := oneOffProgress(ctx, "naming to "+e.targetName)
			img := images.Image{
				Name:      e.targetName,
				Target:    *desc,
				CreatedAt: time.Now(),
			}

			if _, err := e.opt.Images.Update(ctx, img); err != nil {
				if !errdefs.IsNotFound(err) {
					return tagDone(err)
				}

				if _, err := e.opt.Images.Create(ctx, img); err != nil {
					return tagDone(err)
				}
			}
			tagDone(nil)
		}
		if e.push {
			return push.Push(ctx, e.opt.SessionManager, e.opt.ImageWriter.ContentStore(), desc.Digest, e.targetName, e.insecure)
		}
	}

	return nil
}
