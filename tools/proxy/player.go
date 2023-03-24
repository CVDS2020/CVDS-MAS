package main

import (
	"github.com/aler9/gortsplib/v2"
	"github.com/aler9/gortsplib/v2/pkg/url"
)

type Player struct {
	url    *url.URL
	client *gortsplib.Client
}
