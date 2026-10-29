// Package authclient is a thin wrapper over golang.org/x/oauth2 for
// third-party login; it corresponds to Yii2's yii\authclient subsystem.
//
// A Provider covers the three steps of a redirect flow — build the
// authorization URL, exchange the code, fetch the identity behind the token
// — with GitHub and Google shipping built in. The identity the flow ends
// with is the provider's report of who authorized the application
// (authclient.Identity); turning it into the application's own user is
// deliberately outside: the callback finds or creates the local account
// behind Provider+ID and logs in through the regular web.User path, so
// third-party login composes with — instead of parallel to — everything
// already built on web.Identity.
//
// The state parameter's CSRF defense ships as GenerateState / ValidState;
// where the state is kept between redirect and callback (a session value,
// a signed cookie) belongs to the application, matching how the web
// package's CSRF filter leaves storage decisions to its callers.
package authclient
