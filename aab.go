package aab

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/xmxu/aab-parser/pb"
	"google.golang.org/protobuf/proto"

	_ "image/jpeg" // handle jpeg format
	_ "image/png"  // handle png format

	_ "golang.org/x/image/webp" // handle webp format
)

type Manifest struct {
	Package     string
	VersionCode int64
	VersionName string
	App         Application
}

type Application struct {
	Icon  string
	Label string
}

func (a *Application) isFilled() bool {
	return len(a.Icon) > 0 && len(a.Label) > 0
}

type Aab struct {
	f         *os.File
	zipreader *zip.Reader
	manifest  *Manifest
	resource  *pb.Package
}

func OpenFile(filename string) (apk *Aab, err error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			f.Close()
		}
	}()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	apk, err = OpenZipReader(f, fi.Size())
	if err != nil {
		return nil, err
	}
	apk.f = f
	return
}

func OpenZipReader(r io.ReaderAt, size int64) (*Aab, error) {
	zipreader, err := zip.NewReader(r, size)
	if err != nil {
		return nil, err
	}
	apk := &Aab{
		zipreader: zipreader,
		manifest: &Manifest{
			App: Application{},
		},
	}
	if err = apk.parseManifest(); err != nil {
		return nil, err
	}
	if err = apk.parseResources(); err != nil {
		return nil, err
	}

	return apk, nil
}

func (a *Aab) Close() error {
	return a.f.Close()
}

func (a *Aab) parseManifest() error {
	data, err := a.readZipFile("base/manifest/AndroidManifest.xml")
	if err != nil {
		return err
	}
	xmlNode := pb.XmlNode{}
	err = proto.Unmarshal(data, &xmlNode)
	if err != nil {
		return err
	}
	element := xmlNode.GetElement()
	attributes := element.GetAttribute()
	for _, attr := range attributes {
		switch attr.GetName() {
		case "package":
			a.manifest.Package = attr.GetValue()
		case "versionCode":
			if code, err := strconv.ParseInt(attr.GetValue(), 10, 64); err == nil {
				a.manifest.VersionCode = code
			}
		case "versionName":
			a.manifest.VersionName = attr.GetValue()
		}
	}
	children := element.Child
	var childElem *pb.XmlElement
outloop:
	for _, child := range children {
		childElem = child.GetElement()
		if childElem == nil {
			continue
		}
		if childElem.Name == "application" {
			attributes := childElem.Attribute
			for _, attr := range attributes {
				if item := attr.GetCompiledItem(); item != nil {
					if ref := item.GetRef(); ref != nil {
						switch attr.GetName() {
						case "icon":
							a.manifest.App.Icon = ref.GetName()
						case "label":
							a.manifest.App.Label = ref.GetName()
						}
						if a.manifest.App.isFilled() {
							break outloop
						}
					}
				}
			}
		}
	}
	return nil
}

func (a *Aab) parseResources() error {
	data, err := a.readZipFile("base/resources.pb")
	if err != nil {
		return err
	}
	xmlNode := pb.ResourceTable{}
	err = proto.Unmarshal(data, &xmlNode)
	if err != nil {
		return err
	}
	for _, p := range xmlNode.Package {
		if p.PackageName == a.manifest.Package {
			a.resource = p
			break
		}
	}
	return nil
}

func (k *Aab) readZipFile(name string) (data []byte, err error) {
	buf := bytes.NewBuffer(nil)
	for _, file := range k.zipreader.File {
		if file.Name != name {
			continue
		}
		rc, er := file.Open()
		if er != nil {
			err = er
			return
		}
		defer rc.Close()
		_, err = io.Copy(buf, rc)
		if err != nil {
			return
		}
		return buf.Bytes(), nil
	}
	return nil, fmt.Errorf("file %s not found", strconv.Quote(name))
}

func (a *Aab) findResource(t, name string, config *pb.Configuration) string {
	if a.resource == nil {
		return ""
	}
	var value *pb.Value
	for _, tt := range a.resource.Type {
		if tt.Name == t {
			if tt.Entry != nil {
				for _, e := range tt.Entry {
					if e.Name == name {
						for _, c := range e.ConfigValue {
							if matchConfig(config, c.Config) {
								value = c.Value
								break
							}
						}
					}
				}

			}
		}
	}
	if value != nil {
		if item := value.GetItem(); item != nil {
			switch t {
			case "mipmap", "drawable":
				if file := item.GetFile(); file != nil {
					return file.Path
				}
			case "string":
				if str := item.GetStr(); str != nil {
					return str.Value
				}
			}

		}
	}
	return ""
}

func (a *Aab) PackageName() string {
	return a.manifest.Package
}

func (a *Aab) Manifest() *Manifest {
	return a.manifest
}

func (a *Aab) Icon(config *pb.Configuration) (image.Image, error) {
	if len(a.manifest.App.Icon) == 0 {
		return nil, errors.New("not found icon resource")
	}
	parts := strings.Split(a.manifest.App.Icon, "/")
	if len(parts) != 2 {
		return nil, errors.New("invalid icon resource")
	}
	iconPath := a.findResource(parts[0], parts[1], config)
	if len(iconPath) > 0 {
		imageData, err := a.readZipFile("base/" + iconPath)
		if err != nil {
			return nil, err
		}
		m, _, err := image.Decode(bytes.NewReader(imageData))
		return m, err
	}

	return nil, errors.New("not found icon resource")
}

func (a *Aab) Label(config *pb.Configuration) string {
	if len(a.manifest.App.Label) > 0 {
		parts := strings.Split(a.manifest.App.Label, "/")
		if len(parts) != 2 {
			return ""
		}
		return a.findResource(parts[0], parts[1], config)
	}

	return ""
}

func matchConfig(a, b *pb.Configuration) bool {
	if a != nil && a.Density > 0 && a.Density != b.Density {
		return false
	}
	//TODO: support other configurations
	return true
}
