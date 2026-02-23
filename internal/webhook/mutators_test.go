package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	snowplanev1alpha1 "github.com/hupe1980/snowplane/api/v1alpha1"
)

func makeMutateRequest(obj runtime.Object, kind string) admission.Request {
	raw, err := json.Marshal(obj)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal obj: %v", err))
	}

	return admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Create,
			Object:    runtime.RawExtension{Raw: raw},
			Kind:      metav1.GroupVersionKind{Kind: kind},
		},
	}
}

func TestDefaultsMutator_Database_DefaultsApplied(t *testing.T) {
	t.Parallel()

	m := NewDefaultsMutator(testScheme())
	db := &snowplanev1alpha1.Database{
		TypeMeta:   metav1.TypeMeta{APIVersion: "snowplane.hupe1980.github.io/v1alpha1", Kind: "Database"},
		ObjectMeta: metav1.ObjectMeta{Name: "test-db", Namespace: "default"},
		Spec:       snowplanev1alpha1.DatabaseSpec{Name: "MYDB"},
	}

	resp := m.Handle(context.Background(), makeMutateRequest(db, "Database"))

	require.True(t, resp.Allowed)
	assert.NotEmpty(t, resp.Patches, "should have patches for defaults")
}

func TestDefaultsMutator_Database_NoOverwrite(t *testing.T) {
	t.Parallel()

	m := NewDefaultsMutator(testScheme())
	db := &snowplanev1alpha1.Database{
		TypeMeta:   metav1.TypeMeta{APIVersion: "snowplane.hupe1980.github.io/v1alpha1", Kind: "Database"},
		ObjectMeta: metav1.ObjectMeta{Name: "test-db", Namespace: "default"},
		Spec: snowplanev1alpha1.DatabaseSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyOrphan,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "custom"},
			},
			Name: "MYDB",
		},
	}

	resp := m.Handle(context.Background(), makeMutateRequest(db, "Database"))

	require.True(t, resp.Allowed)
}

func TestDefaultsMutator_User_TypeDefaultsToPerson(t *testing.T) {
	t.Parallel()

	m := NewDefaultsMutator(testScheme())
	user := &snowplanev1alpha1.User{
		TypeMeta:   metav1.TypeMeta{APIVersion: "snowplane.hupe1980.github.io/v1alpha1", Kind: "User"},
		ObjectMeta: metav1.ObjectMeta{Name: "test-user", Namespace: "default"},
		Spec:       snowplanev1alpha1.UserSpec{Name: "MYUSER"},
	}

	resp := m.Handle(context.Background(), makeMutateRequest(user, "User"))

	require.True(t, resp.Allowed)
	assert.NotEmpty(t, resp.Patches, "should have patches for type default")
}

func TestDefaultsMutator_User_TypeNotOverwritten(t *testing.T) {
	t.Parallel()

	m := NewDefaultsMutator(testScheme())
	svcType := snowplanev1alpha1.UserTypeService
	user := &snowplanev1alpha1.User{
		TypeMeta:   metav1.TypeMeta{APIVersion: "snowplane.hupe1980.github.io/v1alpha1", Kind: "User"},
		ObjectMeta: metav1.ObjectMeta{Name: "test-user", Namespace: "default"},
		Spec: snowplanev1alpha1.UserSpec{
			CommonSpec: snowplanev1alpha1.CommonSpec{
				DeletionPolicy: snowplanev1alpha1.DeletionPolicyDelete,
				ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "default"},
			},
			Name: "MYUSER",
			Type: &svcType,
		},
	}

	resp := m.Handle(context.Background(), makeMutateRequest(user, "User"))

	require.True(t, resp.Allowed)
}

func TestDefaultsMutator_Schema_DefaultsApplied(t *testing.T) {
	t.Parallel()

	m := NewDefaultsMutator(testScheme())
	schema := &snowplanev1alpha1.Schema{
		TypeMeta:   metav1.TypeMeta{APIVersion: "snowplane.hupe1980.github.io/v1alpha1", Kind: "Schema"},
		ObjectMeta: metav1.ObjectMeta{Name: "test-schema", Namespace: "default"},
		Spec: snowplanev1alpha1.SchemaSpec{
			Name:        "MYSCHEMA",
			DatabaseRef: &snowplanev1alpha1.LocalObjectReference{Name: "mydb"},
		},
	}

	resp := m.Handle(context.Background(), makeMutateRequest(schema, "Schema"))

	require.True(t, resp.Allowed)
	assert.NotEmpty(t, resp.Patches)
}

func TestDefaultsMutator_Warehouse_DefaultsApplied(t *testing.T) {
	t.Parallel()

	m := NewDefaultsMutator(testScheme())
	wh := &snowplanev1alpha1.Warehouse{
		TypeMeta:   metav1.TypeMeta{APIVersion: "snowplane.hupe1980.github.io/v1alpha1", Kind: "Warehouse"},
		ObjectMeta: metav1.ObjectMeta{Name: "test-wh", Namespace: "default"},
		Spec:       snowplanev1alpha1.WarehouseSpec{Name: "MYWH"},
	}

	resp := m.Handle(context.Background(), makeMutateRequest(wh, "Warehouse"))

	require.True(t, resp.Allowed)
	assert.NotEmpty(t, resp.Patches)
}

func TestDefaultsMutator_AccountRole_DefaultsApplied(t *testing.T) {
	t.Parallel()

	m := NewDefaultsMutator(testScheme())
	role := &snowplanev1alpha1.AccountRole{
		TypeMeta:   metav1.TypeMeta{APIVersion: "snowplane.hupe1980.github.io/v1alpha1", Kind: "AccountRole"},
		ObjectMeta: metav1.ObjectMeta{Name: "test-role", Namespace: "default"},
		Spec:       snowplanev1alpha1.AccountRoleSpec{Name: "MYROLE"},
	}

	resp := m.Handle(context.Background(), makeMutateRequest(role, "AccountRole"))

	require.True(t, resp.Allowed)
	assert.NotEmpty(t, resp.Patches)
}

func TestDefaultsMutator_UnknownKind_Allowed(t *testing.T) {
	t.Parallel()

	m := NewDefaultsMutator(testScheme())
	req := admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Create,
			Kind:      metav1.GroupVersionKind{Kind: "Unknown"},
		},
	}

	resp := m.Handle(context.Background(), req)

	assert.True(t, resp.Allowed)
}

func TestApplyCommonDefaults_SetsDeletePolicy(t *testing.T) {
	t.Parallel()

	spec := &snowplanev1alpha1.CommonSpec{}
	applyCommonDefaults(spec)

	assert.Equal(t, snowplanev1alpha1.DeletionPolicyDelete, spec.DeletionPolicy)
	assert.Equal(t, "default", spec.ProviderRef.Name)
}

func TestApplyCommonDefaults_PreservesExisting(t *testing.T) {
	t.Parallel()

	spec := &snowplanev1alpha1.CommonSpec{
		DeletionPolicy: snowplanev1alpha1.DeletionPolicyOrphan,
		ProviderRef:    snowplanev1alpha1.ProviderReference{Name: "custom"},
	}

	applyCommonDefaults(spec)

	assert.Equal(t, snowplanev1alpha1.DeletionPolicyOrphan, spec.DeletionPolicy)
	assert.Equal(t, "custom", spec.ProviderRef.Name)
}

func TestDefaultsMutator_AccountRoleGrant_DefaultsApplied(t *testing.T) {
	t.Parallel()

	m := NewDefaultsMutator(testScheme())
	grant := &snowplanev1alpha1.AccountRoleGrant{
		TypeMeta:   metav1.TypeMeta{APIVersion: "snowplane.hupe1980.github.io/v1alpha1", Kind: "AccountRoleGrant"},
		ObjectMeta: metav1.ObjectMeta{Name: "test-grant", Namespace: "default"},
		Spec: snowplanev1alpha1.AccountRoleGrantSpec{
			Privilege:   "USAGE",
			On:          snowplanev1alpha1.GrantOn{Account: true},
			AccountRole: "ROLE1",
		},
	}

	resp := m.Handle(context.Background(), makeMutateRequest(grant, "AccountRoleGrant"))

	require.True(t, resp.Allowed)
	assert.NotEmpty(t, resp.Patches, "should have patches for defaults")
}

func TestDefaultsMutator_Table_DefaultsApplied(t *testing.T) {
	t.Parallel()

	m := NewDefaultsMutator(testScheme())
	table := &snowplanev1alpha1.Table{
		TypeMeta:   metav1.TypeMeta{APIVersion: "snowplane.hupe1980.github.io/v1alpha1", Kind: "Table"},
		ObjectMeta: metav1.ObjectMeta{Name: "test-table", Namespace: "default"},
		Spec: snowplanev1alpha1.TableSpec{
			Name:        "MY_TABLE",
			DatabaseRef: &snowplanev1alpha1.LocalObjectReference{Name: "mydb"},
			SchemaRef:   &snowplanev1alpha1.LocalObjectReference{Name: "myschema"},
			Columns:     []snowplanev1alpha1.ColumnDefinition{{Name: "ID", Type: "NUMBER"}},
		},
	}

	resp := m.Handle(context.Background(), makeMutateRequest(table, "Table"))

	require.True(t, resp.Allowed)
	assert.NotEmpty(t, resp.Patches, "should have patches for defaults")
}

func TestDefaultsMutator_View_DefaultsApplied(t *testing.T) {
	t.Parallel()

	m := NewDefaultsMutator(testScheme())
	view := &snowplanev1alpha1.View{
		TypeMeta:   metav1.TypeMeta{APIVersion: "snowplane.hupe1980.github.io/v1alpha1", Kind: "View"},
		ObjectMeta: metav1.ObjectMeta{Name: "test-view", Namespace: "default"},
		Spec: snowplanev1alpha1.ViewSpec{
			Name:        "MY_VIEW",
			DatabaseRef: &snowplanev1alpha1.LocalObjectReference{Name: "mydb"},
			SchemaRef:   &snowplanev1alpha1.LocalObjectReference{Name: "myschema"},
			Statement:   "SELECT 1",
		},
	}

	resp := m.Handle(context.Background(), makeMutateRequest(view, "View"))

	require.True(t, resp.Allowed)
	assert.NotEmpty(t, resp.Patches, "should have patches for defaults")
}

func TestDefaultsMutator_Stage_DefaultsApplied(t *testing.T) {
	t.Parallel()

	m := NewDefaultsMutator(testScheme())
	stage := &snowplanev1alpha1.Stage{
		TypeMeta:   metav1.TypeMeta{APIVersion: "snowplane.hupe1980.github.io/v1alpha1", Kind: "Stage"},
		ObjectMeta: metav1.ObjectMeta{Name: "test-stage", Namespace: "default"},
		Spec: snowplanev1alpha1.StageSpec{
			Name:        "MY_STAGE",
			DatabaseRef: &snowplanev1alpha1.LocalObjectReference{Name: "mydb"},
			SchemaRef:   &snowplanev1alpha1.LocalObjectReference{Name: "myschema"},
		},
	}

	resp := m.Handle(context.Background(), makeMutateRequest(stage, "Stage"))

	require.True(t, resp.Allowed)
	assert.NotEmpty(t, resp.Patches, "should have patches for defaults")
}
