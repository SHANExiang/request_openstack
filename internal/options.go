package internal

import (
    "fmt"
    "github.com/valyala/fasthttp"
    "request_openstack/configs"
    "request_openstack/utils"
)

type Option func(opts *Options)

type Options struct {
    AdminProject               string
    ProjectId                  string
    Request
    Token                      string
    Snowflake                  *utils.Snowflake
    IsAdmin                    bool
}

func WithRequest(uri string, client *fasthttp.Client) Option {
    return func(opts *Options) {
        opts.Request = Request{
            UrlPrefix: fmt.Sprintf("http://%s:%s", configs.CONF.Host, uri),
            Client: client,
        }
    }
}

func WithProjectId(projectId string) Option {
    return func(opts *Options) {
        opts.ProjectId = projectId
    }
}

func WithAdminProjectId(projectId string) Option {
    return func(opts *Options) {
        opts.AdminProject = projectId
    }
}

func WithToken(token string) Option {
    return func(opts *Options) {
        opts.Token = token
    }
}

func WithSnowFlake() Option {
    return func(opts *Options) {
        opts.Snowflake = utils.NewSnowflake(uint16(1))
    }
}

func WithIsAdmin(isAdmin bool) Option {
    return func(opts *Options) {
        opts.IsAdmin = isAdmin
    }
}
