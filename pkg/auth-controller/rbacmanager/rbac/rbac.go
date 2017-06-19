package rbac

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/pkg/apis/rbac/v1beta1"
)

func GenerateRoleByNamespace(namespace string) *v1beta1.Role {
	policyRule := v1beta1.PolicyRule{
		Verbs:     []string{v1beta1.VerbAll},
		APIGroups: []string{v1beta1.APIGroupAll},
		Resources: []string{v1beta1.ResourceAll},
	}
	role := &v1beta1.Role{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Role",
			APIVersion: "rbac.authorization.k8s.io/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default-role",
			Namespace: namespace,
		},
		Rules: []v1beta1.PolicyRule{policyRule},
	}
	return role
}

func GenerateRoleBinding(namespace, tenant string) *v1beta1.RoleBinding {
	subject := v1beta1.Subject{
		Kind: "Group",
		Name: tenant,
	}
	roleRef := v1beta1.RoleRef{
		APIGroup: "rbac.authorization.k8s.io",
		Kind:     "Role",
		Name:     "default-role",
	}
	roleBinding := &v1beta1.RoleBinding{
		TypeMeta: metav1.TypeMeta{
			Kind:       "RoleBinding",
			APIVersion: "rbac.authorization.k8s.io/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      tenant + "-rolebinding",
			Namespace: namespace,
		},
		Subjects: []v1beta1.Subject{subject},
		RoleRef:  roleRef,
	}
	return roleBinding
}

func GenerateClusterRole() *v1beta1.ClusterRole {
	policyRule := v1beta1.PolicyRule{
		Verbs:     []string{v1beta1.VerbAll},
		APIGroups: []string{v1beta1.APIGroupAll},
		Resources: []string{"namespaces"},
	}

	clusterRole := &v1beta1.ClusterRole{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterRole",
			APIVersion: "rbac.authorization.k8s.io/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "namespace-creater",
		},
		Rules: []v1beta1.PolicyRule{policyRule},
	}
	return clusterRole
}

func GenerateClusterRoleBindingByTenant(tenant string) *v1beta1.ClusterRoleBinding {
	subject := v1beta1.Subject{
		Kind: "Group",
		Name: tenant,
	}
	roleRef := v1beta1.RoleRef{
		APIGroup: "rbac.authorization.k8s.io",
		Kind:     "ClusterRole",
		Name:     "namespace-creater",
	}

	clusterRoleBinding := &v1beta1.ClusterRoleBinding{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterRoleBinding",
			APIVersion: "rbac.authorization.k8s.io/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: tenant + "-namespace-creater",
		},
		Subjects: []v1beta1.Subject{subject},
		RoleRef:  roleRef,
	}
	return clusterRoleBinding
}
