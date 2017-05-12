package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"net/http"
	"path"
	"strings"
	"text/template"

	"github.com/goreleaser/goreleaser/config"
)
var tplsrc=`#!/bin/sh
set -e
#  Code generated by godownloader. DO NOT EDIT.
#

usage() {
  this=$1
  cat <<EOF
$this: download go binaries for {{ $.Release.GitHub.Owner }}/{{ $.Release.GitHub.Name }}

Usage: $this [version]
  where [version] is 'latest' or a version number from
  https://github.com/{{ $.Release.GitHub.Owner }}/{{ $.Release.GitHub.Name }}/releases

Generated by godownloader
 https://github.com/goreleaser/godownloader

EOF
}
` + shellfn + `
OWNER={{ $.Release.GitHub.Owner }}
REPO={{ $.Release.GitHub.Name }}
BINARY={{ .Build.Binary }}
FORMAT={{ .Archive.Format }}
BINDIR=${BINDIR:-./bin}

VERSION=$1
if [ -z "${VERSION}" ]; then
  usage $0
  exit 1
fi

if [ "${VERSION}" = "latest" ]; then
  echo "Checking GitHub for latest version of ${OWNER}/${REPO}"
  VERSION=$(github_last_release "$OWNER/$REPO")
fi
# if version starts with 'v', remove it
VERSION=${VERSION#v}

OS=$(uname_os)
ARCH=$(uname_arch)

# change format (tar.gz or zip) based on ARCH
{{- with .Archive.FormatOverrides }}
case ${ARCH} in
{{- range . }}
{{ .Goos }}) FORMAT={{ .Format }} ;;
esac
{{- end }}
{{- end }}

# adjust archive name based on OS
{{- with .Archive.Replacements }}
case ${OS} in
{{- range $k, $v := . }}
{{ $k }}) OS={{ $v }} ;;
{{- end }}
esac

# adjust archive name based on ARCH
case ${ARCH} in
{{- range $k, $v := . }}
{{ $k }}) ARCH={{ $v }} ;;
{{- end }}
esac
{{- end }}

{{ .Archive.NameTemplate }}
TARBALL=${NAME}.${FORMAT}
TARBALL_URL=https://github.com/${OWNER}/${REPO}/releases/download/v${VERSION}/${TARBALL}
CHECKSUM=${REPO}_checksums.txt
CHECKSUM_URL=https://github.com/${OWNER}/${REPO}/releases/download/v${VERSION}/${CHECKSUM}

# Destructive operations start here
#
#
TMPDIR=$(mktmpdir)
http_download ${TMPDIR}/${TARBALL} ${TARBALL_URL}

# checksum goes here
if [ 1 -eq 1 ]; then
  http_download ${TMPDIR}/${CHECKSUM} ${CHECKSUM_URL}
  hash_sha256_verify ${TMPDIR}/${TARBALL} ${TMPDIR}/${CHECKSUM}
fi

(cd ${TMPDIR} && untar ${TARBALL})
install -d ${BINDIR}
install ${TMPDIR}/${BINARY} ${BINDIR}/
`

func makeShell(cfg *config.Project) (string, error) {
	var out bytes.Buffer
	t, err := template.New("shell").Parse(tplsrc)
	if err != nil {
		return "", err
	}
	err = t.Execute(&out, cfg)
	return out.String(), err
}

// converts the given name template to it's equivalent in shell
// except for the default goreleaser templates, templates with
// conditionals will return an error
//
// {{ .Binary }} --->  NAME=${BINARY}, etc.
//
func makeName(target string) (string, error) {
	prefix := ""
	// TODO: error on conditionals
	if target == "" || target == "{{ .Binary }}_{{ .Os }}_{{ .Arch }}{{ if .Arm }}v{{ .Arm }}{{ end }}" {
		prefix = "if [ ! -z \"${ARM}\" ]; then ARM=\"v$ARM\"; fi"
		target = "{{ .Binary }}_{{ .Os }}_{{ .Arch }}{{ .Arm }}"
	}
	var varmap = map[string]string{
		"Os":      "${OS}",
		"Arch":    "${ARCH}",
		"Arm":     "${ARM}",
		"Version": "${VERSION}",
		"Tag":     "${TAG}",
		"Binary":  "${BINARY}",
	}

	var out bytes.Buffer
	if prefix != "" {
		out.WriteString(prefix + "\n")
	}
	out.WriteString("NAME=")
	t, err := template.New("name").Parse(target)
	if err != nil {
		return "", err
	}
	err = t.Execute(&out, varmap)
	return out.String(), err
}

func loadURL(file string) (*config.Project, error) {
	resp, err := http.Get(file)
	if err != nil {
		return nil, err
	}
	p, err := config.LoadReader(resp.Body)

	// to make errcheck happy
	errc := resp.Body.Close()
	if errc != nil {
		return nil, errc
	}
	return &p, err
}

func loadFile(file string) (*config.Project, error) {
	p, err := config.Load(file)
	return &p, err
}

// Load project configuration from a given repo name or filepath/url.
func Load(repo string, file string) (project *config.Project, err error) {
	if repo == "" && file == "" {
		return nil, fmt.Errorf("Need a repo or file")
	}
	if file == "" {
		file = "https://raw.githubusercontent.com/" + repo + "/master/goreleaser.yml"
	}

	log.Printf("Reading %s", file)
	if strings.HasPrefix(file, "http") {
		project, err = loadURL(file)
	} else {
		project, err = loadFile(file)
	}
	if err != nil {
		return nil, err
	}

	// if not specified add in GitHub owner/repo info
	if project.Release.GitHub.Owner == "" {
		if repo == "" {
			return nil, fmt.Errorf("need to provide owner/name repo")
		}
		project.Release.GitHub.Owner = path.Dir(repo)
		project.Release.GitHub.Name = path.Base(repo)
	}

	// set default archive format
	if project.Archive.Format == "" {
		project.Archive.Format = "tar.gz"
	}

	// set default binary name
	if project.Build.Binary == "" {
		project.Build.Binary = path.Base(repo)
	}

	return project, nil
}

func main() {
	repo := flag.String("repo", "", "owner/name of repository")
	flag.Parse()
	args := flag.Args()
	file := ""
	if len(args) > 0 {
		file = args[0]
	}
	cfg, err := Load(*repo, file)
	if err != nil {
		log.Fatalf("Unable to parse: %s", err)
	}

	// get name template
	name, err := makeName(cfg.Archive.NameTemplate)
	cfg.Archive.NameTemplate = name
	if err != nil {
		log.Fatalf("Unable generate name: %s", err)
	}

	shell, err := makeShell(cfg)
	if err != nil {
		log.Fatalf("Unable to generate shell: %s", err)
	}
	fmt.Println(shell)
}
