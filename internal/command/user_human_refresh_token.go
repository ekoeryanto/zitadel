package command

import (
	"context"
	"time"

	"github.com/caos/zitadel/internal/domain"
	caos_errs "github.com/caos/zitadel/internal/errors"
	"github.com/caos/zitadel/internal/eventstore"
	"github.com/caos/zitadel/internal/repository/user"
)

func (c *Commands) AddUserAndRefreshToken(ctx context.Context, orgID, agentID, clientID, userID, refreshToken string, audience, scopes, authMethodsReferences []string, lifetime time.Duration, authTime time.Time) (*domain.Token, string, error) {
	userWriteModel := NewUserWriteModel(userID, orgID)
	accessTokenEvent, accessToken, err := c.addUserToken(ctx, userWriteModel, agentID, clientID, audience, scopes, lifetime)
	if err != nil {
		return nil, "", err
	}

	creator := func() (eventstore.EventPusher, string, error) {
		return c.addRefreshToken(ctx, accessToken, authMethodsReferences, authTime, 5*time.Minute, 20*time.Minute)
	} //TODO: duration
	if refreshToken != "" {
		creator = func() (eventstore.EventPusher, string, error) {
			return c.renewRefreshToken(ctx, userID, orgID, refreshToken, 5*time.Minute)
		}
	}
	refreshTokenEvent, token, err := creator()
	if err != nil {
		return nil, "", err
	}
	_, err = c.eventstore.PushEvents(ctx, accessTokenEvent, refreshTokenEvent)
	if err != nil {
		return nil, "", err
	}
	return accessToken, token, nil
}

func (c *Commands) addRefreshToken(ctx context.Context, accessToken *domain.Token, authMethodsReferences []string, authTime time.Time, idleExpiration, expiration time.Duration) (*user.HumanRefreshTokenAddedEvent, string, error) {
	//if userID == "" {
	//	return nil, caos_errs.ThrowInvalidArgument(nil, "COMMAND-GVDg2", "Errors.IDMissing")
	//}
	//
	//existingHuman, err := c.getHumanWriteModelByID(ctx, userID, orgID)
	//if err != nil {
	//	return nil, err
	//}
	//if !isUserStateExists(existingHuman.UserState) {
	//	return nil, caos_errs.ThrowNotFound(nil, "COMMAND-Dgf2w", "Errors.User.NotFound")
	//}
	//
	////audience = domain.AddAudScopeToAudience(audience, scopes)
	//
	////preferredLanguage := ""
	////existingHuman, err := c.getHumanWriteModelByID(ctx, userID, orgID)
	////if existingHuman != nil {
	////	preferredLanguage = existingHuman.PreferredLanguage.String()
	////}
	tokenID, err := c.idGenerator.Next()
	if err != nil {
		return nil, "", err
	}
	refreshToken, err := domain.NewRefreshToken(accessToken.AggregateID, tokenID, c.keyAlgorithm)
	if err != nil {
		return nil, "", err
	}
	refreshTokenWriteModel := NewHumanRefreshTokenWriteModel(accessToken.AggregateID, accessToken.ResourceOwner, tokenID)
	userAgg := UserAggregateFromWriteModel(&refreshTokenWriteModel.WriteModel)
	return user.NewHumanRefreshTokenAddedEvent(ctx, userAgg, tokenID, accessToken.ApplicationID, accessToken.UserAgentID,
			accessToken.PreferredLanguage, accessToken.Audience, accessToken.Scopes, authMethodsReferences, authTime, idleExpiration, expiration),
		refreshToken, nil
}

func (c *Commands) renewRefreshToken(ctx context.Context, userID, orgID, refreshToken string, idleExpiration time.Duration) (event *user.HumanRefreshTokenRenewedEvent, newRefreshToken string, err error) {
	if refreshToken == "" {
		return nil, "", caos_errs.ThrowInvalidArgument(nil, "COMMAND-DHrr3", "Errors.IDMissing")
	}

	tokenUserID, tokenID, token, err := domain.FromRefreshToken(refreshToken, c.keyAlgorithm)
	if err != nil {
		return nil, "", err
	}
	if tokenUserID != userID {
		return nil, "", caos_errs.ThrowNotFound(nil, "COMMAND-Ht2g2", "Errors.User.RefreshToken.Invalid")
	}
	refreshTokenWriteModel := NewHumanRefreshTokenWriteModel(userID, orgID, tokenID)
	err = c.eventstore.FilterToQueryReducer(ctx, refreshTokenWriteModel)
	if err != nil {
		return nil, "", err
	}
	if refreshTokenWriteModel.UserState != domain.UserStateActive {
		return nil, "", caos_errs.ThrowNotFound(nil, "COMMAND-BHnhs", "Errors.User.RefreshToken.Invalid")
	}
	if refreshTokenWriteModel.RefreshToken != token ||
		refreshTokenWriteModel.IdleExpiration.Before(time.Now()) ||
		refreshTokenWriteModel.Expiration.Before(time.Now()) {
		return nil, "", caos_errs.ThrowInvalidArgument(nil, "COMMAND-Vr43e", "Errors.User.RefreshToken.Invalid")
	}

	newToken, err := c.idGenerator.Next()
	if err != nil {
		return nil, "", err
	}
	newRefreshToken, err = domain.RefreshToken(userID, tokenID, newToken, c.keyAlgorithm)
	if err != nil {
		return nil, "", err
	}
	userAgg := UserAggregateFromWriteModel(&refreshTokenWriteModel.WriteModel)
	return user.NewHumanRefreshTokenRenewedEvent(ctx, userAgg, tokenID, newToken, idleExpiration), newRefreshToken, nil
}
