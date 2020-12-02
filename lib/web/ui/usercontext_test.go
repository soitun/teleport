package ui

import (
	"testing"

	"github.com/gravitational/teleport/lib/defaults"
	"github.com/gravitational/teleport/lib/services"
	"gopkg.in/check.v1"
)

type UserContextSuite struct{}

var _ = check.Suite(&UserContextSuite{})

func TestUserContext(t *testing.T) { check.TestingT(t) }

func (s *UserContextSuite) TestNewUserContext(c *check.C) {
	user := &services.UserV2{
		Metadata: services.Metadata{
			Name: "root",
		},
	}

	// set some rules
	role1 := &services.RoleV3{}
	role1.SetNamespaces(services.Allow, []string{defaults.Namespace})
	role1.SetRules(services.Allow, []services.Rule{
		{
			Resources: []string{services.KindAuthConnector},
			Verbs:     services.RW(),
		},
	})

	// not setting the rule, or explicitly denying, both denies access
	role1.SetRules(services.Deny, []services.Rule{
		{
			Resources: []string{services.KindEvent},
			Verbs:     services.RW(),
		},
	})

	role2 := &services.RoleV3{}
	role2.SetNamespaces(services.Allow, []string{defaults.Namespace})
	role2.SetRules(services.Allow, []services.Rule{
		{
			Resources: []string{services.KindTrustedCluster},
			Verbs:     services.RW(),
		},
	})

	// set some logins
	role1.SetLogins(services.Allow, []string{"a", "b"})
	role1.SetLogins(services.Deny, []string{"c"})
	role2.SetLogins(services.Allow, []string{"d"})

	roleSet := []services.Role{role1, role2}
	userContext, err := NewUserContext(user, roleSet)
	c.Assert(err, check.IsNil)

	allowed := access{true, true, true, true, true}
	denied := access{false, false, false, false, false}

	// test user name and acl
	c.Assert(userContext.Name, check.Equals, "root")
	c.Assert(userContext.ACL.AuthConnectors, check.DeepEquals, allowed)
	c.Assert(userContext.ACL.TrustedClusters, check.DeepEquals, allowed)
	c.Assert(userContext.ACL.AppServers, check.DeepEquals, denied)
	c.Assert(userContext.ACL.Events, check.DeepEquals, denied)
	c.Assert(userContext.ACL.Sessions, check.DeepEquals, denied)
	c.Assert(userContext.ACL.Roles, check.DeepEquals, denied)
	c.Assert(userContext.ACL.Users, check.DeepEquals, denied)
	c.Assert(userContext.ACL.Tokens, check.DeepEquals, denied)
	c.Assert(userContext.ACL.Nodes, check.DeepEquals, denied)
	c.Assert(userContext.ACL.AccessRequests, check.DeepEquals, denied)
	c.Assert(userContext.ACL.SSHLogins, check.DeepEquals, []string{"a", "b", "d"})
	c.Assert(userContext.AccessStrategy, check.DeepEquals, accessStrategy{
		Type:   services.RequestStrategyOptional,
		Prompt: "",
	})

	// test local auth type
	c.Assert(userContext.AuthType, check.Equals, authLocal)

	// test sso auth type
	user.Spec.GithubIdentities = []services.ExternalIdentity{{ConnectorID: "foo", Username: "bar"}}
	userContext, err = NewUserContext(user, roleSet)
	c.Assert(err, check.IsNil)
	c.Assert(userContext.AuthType, check.Equals, authSSO)
}

func (s *UserContextSuite) TestGetRolesRequestable(c *check.C) {
	role1 := &services.RoleV3{}

	// Test empty state combinations.
	roles := GetRolesRequestable(nil, nil)
	c.Assert(roles, check.DeepEquals, []string{})

	roles = GetRolesRequestable(nil, []string{"foo"})
	c.Assert(roles, check.DeepEquals, []string{})

	roles = GetRolesRequestable([]services.Role{role1}, []string{"foo"})
	c.Assert(roles, check.DeepEquals, []string{})

	role1.SetAccessRequestConditions(services.Allow, services.AccessRequestConditions{
		Roles: []string{"r1", "r2", "r3"},
	})
	role1.SetAccessRequestConditions(services.Deny, services.AccessRequestConditions{
		Roles: []string{"r4"},
	})

	roles = GetRolesRequestable([]services.Role{role1}, nil)
	c.Assert(roles, check.DeepEquals, []string{"r1", "r2", "r3"})

	// Test for duplicate roles.
	role2 := &services.RoleV3{}
	role2.SetAccessRequestConditions(services.Allow, services.AccessRequestConditions{
		Roles: []string{"r1", "r5"},
	})
	role2.SetAccessRequestConditions(services.Deny, services.AccessRequestConditions{
		Roles: []string{"r2"},
	})

	// Test that deny trumps allow.
	role3 := &services.RoleV3{}
	role3.SetAccessRequestConditions(services.Allow, services.AccessRequestConditions{
		Roles: []string{"r2", "r4"},
	})

	roleSet := []services.Role{role1, role2, role3}
	roles = GetRolesRequestable(roleSet, nil)
	c.Assert(roles, check.DeepEquals, []string{"r1", "r3", "r5"})

	// Test assumed roles are not part of requestable roles.
	roles = GetRolesRequestable(roleSet, []string{"r1", "r3"})
	c.Assert(roles, check.DeepEquals, []string{"r5"})

	roles = GetRolesRequestable(roleSet, []string{"r1", "r3", "r5"})
	c.Assert(roles, check.DeepEquals, []string{})
}
