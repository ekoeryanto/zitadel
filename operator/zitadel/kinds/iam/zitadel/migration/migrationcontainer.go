package migration

import (
	"fmt"
	"strings"

	"github.com/caos/zitadel/pkg/databases/db"

	"github.com/caos/zitadel/operator/common"

	corev1 "k8s.io/api/core/v1"
)

func getMigrationContainer(
	dbConn db.Connection,
	customImageRegistry string,
	certsVolumeMount corev1.VolumeMount,
	users []string,
	pwSecretName string,
) corev1.Container {

	return corev1.Container{
		Name:  "db-migration",
		Image: common.FlywayImage.Reference(customImageRegistry),
		Args: []string{
			fmt.Sprintf("-url=%s", connectionURL(dbConn, certsVolumeMount.MountPath)),
			fmt.Sprintf("-locations=filesystem:%s", migrationsPath),
			"migrate",
		},
		Env: migrationEnvVars(envMigrationUser, envMigrationPW, dbConn.User(), pwSecretName, users),
		VolumeMounts: []corev1.VolumeMount{certsVolumeMount, {
			Name:      migrationConfigmap,
			MountPath: migrationsPath,
		}},
		TerminationMessagePath:   corev1.TerminationMessagePathDefault,
		TerminationMessagePolicy: "File",
		ImagePullPolicy:          "IfNotPresent",
	}
}

func migrationEnvVars(envMigrationUser, envMigrationPW, migrationUser, userPasswordsSecret string, users []string) []corev1.EnvVar {
	envVars := baseEnvVars(envMigrationUser, envMigrationPW, migrationUser, userPasswordsSecret)

	vars := make([]corev1.EnvVar, 0)
	for _, v := range envVars {
		vars = append(vars, v)
	}

	for _, user := range users {
		vars = append(vars, corev1.EnvVar{
			Name: "FLYWAY_PLACEHOLDERS_" + strings.ToUpper(user) + "PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: userPasswordsSecret},
					Key:                  user,
				},
			},
		})
	}

	return vars
}
