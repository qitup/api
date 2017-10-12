package controllers

import "dubclan/api/models"

func CompleteUserAuth(assume_identity models.Identity) (models.User, error) {
	// Has jwt token for existing user?
	// Yes ->
	// 		Different provider?
	// 		Yes -> Login/Refresh Access token
	// 		No -> Get existing user and save new identity
	// No ->
	// 		Does identity's email collide with identity for an existing user?
	// 		Yes ->
	// 			Different provider?
	// 				Yes -> Get existing user and save new identity
	// 				No -> Login/Refresh Access token
	// 		No -> register new user in store
}
