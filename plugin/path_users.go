package keystoneauth

import (
	"fmt"
	"github.com/hashicorp/vault/logical"
	"github.com/hashicorp/vault/logical/framework"
	"github.com/hashicorp/vault/plugins/helper/database/credsutil"
)

func pathListUsers(b *backend) *framework.Path {
	return &framework.Path{
		Pattern: "users/?$",

		Callbacks: map[logical.Operation]framework.OperationFunc{
			logical.ListOperation: b.pathUserList,
		},
	}
}

func pathUsers(b *backend) *framework.Path {
	return &framework.Path{
		Pattern: "users/" + framework.GenericNameRegex("name"),
		Fields: map[string]*framework.FieldSchema{
			"name": &framework.FieldSchema{
				Type:        framework.TypeString,
				Description: "name",
			},
			"default_project_id": &framework.FieldSchema{
				Type:        framework.TypeString,
				Description: "default_project_id",
				Default:     "",
			},
			"domain_id": &framework.FieldSchema{
				Type:        framework.TypeString,
				Description: "domain_id",
				Default:     "",
			},
			"enabled": &framework.FieldSchema{
				Type:        framework.TypeBool,
				Description: "enabled",
				Default:     true,
			},
			"password": &framework.FieldSchema{
				Type:        framework.TypeString,
				Description: "password",
				Default:     "",
			},
		},
		Callbacks: map[logical.Operation]framework.OperationFunc{
			logical.UpdateOperation: b.pathUserWrite,
			logical.ReadOperation:   b.pathUserRead,
		},
	}
}

func pathUsersEC2(b *backend) *framework.Path {
	return &framework.Path{
		Pattern: "users/" + framework.GenericNameRegex("name")+"/credentials/OS-EC2",
		Fields: map[string]*framework.FieldSchema{
			"name": &framework.FieldSchema{
				Type:        framework.TypeString,
				Description: "name",
			},
			"user_id": &framework.FieldSchema{
				Type:        framework.TypeString,
				Description: "user_id",
			},
			"tenant_id": &framework.FieldSchema{
				Type:        framework.TypeString,
				Description: "tenant_id",
				Default:     "",
			},
		},
		Callbacks: map[logical.Operation]framework.OperationFunc{
			logical.UpdateOperation: b.pathUserEC2Write,
		},
	}
}

func (b *backend) User(s logical.Storage, n string) (*userEntry, error) {
	entry, err := s.Get("user/" + n)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, nil
	}

	var result userEntry

	if err := entry.DecodeJSON(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

func (b *backend) pathUserRead(
	req *logical.Request, data *framework.FieldData) (*logical.Response, error) {

	var namepostfix string
	default_project_id := data.Get("default_project_id").(string)
	namepostfix, _ = credsutil.RandomAlphaNumeric(20, true)
	name := data.Get("name").(string)
	password := data.Get("password").(string)
	domain_id := data.Get("domain_id").(string)
	enabled := data.Get("enabled").(bool)

	user, err := b.User(req.Storage, name)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return logical.ErrorResponse(fmt.Sprintf("unknown user: %s", name)), nil
	}

	conf, err2 := getconfig(req)

	if err2 != nil {
		return nil, fmt.Errorf("configure the Keystone connection with config/connection first")
	}

	name = fmt.Sprintf("%s_%s_%s ","vault", name, namepostfix[4:20])
	password, _ = credsutil.RandomAlphaNumeric(44, true)
	password = password[4:44]
	keystone_url := conf[0]
	token := conf[1]

	//Create the user

	created_usr, err3 := CreateUser(default_project_id, name, password, enabled, token, domain_id, keystone_url)
	created_usr_id := created_usr[1]

	if err3 != nil {
		return nil, fmt.Errorf("creation of the user failed")
	}

	return &logical.Response{
		Data: map[string]interface{}{
			"name":               name,
			"default_project_id": user.User_default_project_id,
			"domain_id":          user.User_domain_id,
			"enabled":            user.User_enabled,
			"password":           password,
			"id":									created_usr_id,
		},
	}, nil
}

func (b *backend) pathUserList(
	req *logical.Request, d *framework.FieldData) (*logical.Response, error) {
	entries, err := req.Storage.List("user/")
	if err != nil {
		return nil, err
	}

	return logical.ListResponse(entries), nil
}

func (b *backend) pathUserWrite(
	req *logical.Request, data *framework.FieldData) (*logical.Response, error) {

	name := data.Get("name").(string)
	default_project_id := data.Get("default_project_id").(string)
	domain_id := data.Get("domain_id").(string)
	enabled := data.Get("enabled").(bool)
	password := data.Get("password").(string)

	// Store it
	entry, err := logical.StorageEntryJSON("user/"+name, &userEntry{
		User_name:               name,
		User_default_project_id: default_project_id,
		User_domain_id:          domain_id,
		User_enabled:            enabled,
		User_password:           password,
	})

	if err != nil {
		return nil, err
	}

	if err != nil {
		return nil, err
	}
	if err := req.Storage.Put(entry); err != nil {
		return nil, err
	}

	return &logical.Response{
		Data: map[string]interface{}{
			"name":               name,
			"created":						true,
		},
	}, nil
}

func (b *backend) pathUserEC2Write(
	req *logical.Request, data *framework.FieldData) (*logical.Response, error) {

	id := data.Get("user_id").(string)
	tenant_id := data.Get("tenant_id").(string)

	conf, err := getconfig(req)

	if err != nil {
		return nil, err
	}

	keystone_url := conf[0]
	token := conf[1]


	// make a request to Keystone

	usrec2, errusrec2 := UserEC2(id, tenant_id, token, keystone_url)

	if errusrec2 != nil {
		return nil, errusrec2
	}

	access_key := usrec2[0]
	secret_key := usrec2[1]

	return &logical.Response{
		Data: map[string]interface{}{
			"access_key": access_key,
			"secret_key": secret_key,
		},
	}, nil
}

type userEntry struct {
	User_name               string `json:"name" structs:"name" mapstructure:"name"`
	User_default_project_id string `json:"default_project_id" structs:"default_project_id" mapstructure:"default_project_id"`
	User_domain_id          string `json:"domain_id" structs:"domain_id" mapstructure:"domain_id"`
	User_enabled            bool   `json:"enabled" structs:"enabled" mapstructure:"enabled"`
	User_password           string `json:"password" structs:"password" mapstructure:"password"`
}
