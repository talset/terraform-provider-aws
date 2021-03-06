package aws

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/iam"

	"github.com/hashicorp/terraform-plugin-sdk/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
)

func resourceAwsIamGroupMembership() *schema.Resource {
	return &schema.Resource{
		Create: resourceAwsIamGroupMembershipCreate,
		Read:   resourceAwsIamGroupMembershipRead,
		Update: resourceAwsIamGroupMembershipUpdate,
		Delete: resourceAwsIamGroupMembershipDelete,
		Importer: &schema.ResourceImporter{
			State: resourceAwsIamGroupMembershipImport,
		},

		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"users": {
				Type:     schema.TypeSet,
				Required: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
				Set:      schema.HashString,
			},

			"group": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
		},
	}
}

func resourceAwsIamGroupMembershipCreate(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*AWSClient).iamconn

	group := d.Get("group").(string)
	userList := expandStringList(d.Get("users").(*schema.Set).List())

	if err := addUsersToGroup(conn, userList, group); err != nil {
		return err
	}

	d.SetId(d.Get("name").(string))
	return resourceAwsIamGroupMembershipRead(d, meta)
}

func resourceAwsIamGroupMembershipRead(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*AWSClient).iamconn
	group := d.Get("group").(string)

	var ul []string
	var marker *string
	for {
		resp, err := conn.GetGroup(&iam.GetGroupInput{
			GroupName: aws.String(group),
			Marker:    marker,
		})

		if err != nil {
			if awsErr, ok := err.(awserr.Error); ok {
				// aws specific error
				if awsErr.Code() == "NoSuchEntity" {
					// group not found
					d.SetId("")
					return nil
				}
			}
			return err
		}

		for _, u := range resp.Users {
			ul = append(ul, *u.UserName)
		}

		if !*resp.IsTruncated {
			break
		}
		marker = resp.Marker
	}

	if err := d.Set("users", ul); err != nil {
		return fmt.Errorf("Error setting user list from IAM Group Membership (%s), error: %s", group, err)
	}

	return nil
}

func resourceAwsIamGroupMembershipUpdate(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*AWSClient).iamconn

	if d.HasChange("users") {
		group := d.Get("group").(string)

		o, n := d.GetChange("users")
		if o == nil {
			o = new(schema.Set)
		}
		if n == nil {
			n = new(schema.Set)
		}

		os := o.(*schema.Set)
		ns := n.(*schema.Set)
		remove := expandStringList(os.Difference(ns).List())
		add := expandStringList(ns.Difference(os).List())

		if err := removeUsersFromGroup(conn, remove, group); err != nil {
			return err
		}

		if err := addUsersToGroup(conn, add, group); err != nil {
			return err
		}
	}

	return resourceAwsIamGroupMembershipRead(d, meta)
}

func resourceAwsIamGroupMembershipDelete(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*AWSClient).iamconn
	userList := expandStringList(d.Get("users").(*schema.Set).List())
	group := d.Get("group").(string)

	err := removeUsersFromGroup(conn, userList, group)
	return err
}

func removeUsersFromGroup(conn *iam.IAM, users []*string, group string) error {
	for _, u := range users {
		_, err := conn.RemoveUserFromGroup(&iam.RemoveUserFromGroupInput{
			UserName:  u,
			GroupName: aws.String(group),
		})

		if err != nil {
			if iamerr, ok := err.(awserr.Error); ok && iamerr.Code() == "NoSuchEntity" {
				return nil
			}
			return err
		}
	}
	return nil
}

func addUsersToGroup(conn *iam.IAM, users []*string, group string) error {
	for _, u := range users {
		_, err := conn.AddUserToGroup(&iam.AddUserToGroupInput{
			UserName:  u,
			GroupName: aws.String(group),
		})

		if err != nil {
			return err
		}
	}
	return nil
}

func resourceAwsIamGroupMembershipImport(d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	groupName := d.Id()
	if len(groupName) == 0 {
		return nil, fmt.Errorf("unexpected format of ID (%q), expected <group-name>", d.Id())
	}

	d.Set("group", groupName)
	d.SetId(resource.UniqueId())

	return []*schema.ResourceData{d}, nil
}
