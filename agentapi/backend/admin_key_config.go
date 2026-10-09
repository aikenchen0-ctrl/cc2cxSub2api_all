package main

// builtinMainAdminAPIKey is the main-site administrator key used when the
// deployment did not inject a real SUB2API_ADMIN_API_KEY. The station owner
// created by one-click open is that main-site user; balance reads use this
// key only on the AgentAPI server and never send it to the browser.
const builtinMainAdminAPIKey = "admin-4eb1ffc783ba3209134fbcb425c473dce871957b66c2a267bc3c91448240947f"
