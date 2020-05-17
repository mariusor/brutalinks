## The "show me more" release 

Some of the features I worked on this past month are:

* Users can invite other people on the platform. This is intended to be optional for instances that don't plan of having registrations open.

* I added a new user listing page, that can be found at [littr.me/~](https://littr.me/~). This page shows the users in a threaded fashion, even though I haven't created the CSS for properly displaying the invitations tree.

* I've fixed the issue when a user would be able to click on the star (Follow) icon on a user profile multiple times.

* I've added a link pointing to the underlying ActivityPub object IRI when hovering over an item or account.

* I've fixed an issue that prevented child comments loading all their replies when viewing them. Previously we loaded only the first level, now we see everything lower in the thread.

* When replying to a thread, all grandparents' authors are added to the CC and receive a notification. This might not be a good idea. :)

* I've moved away from using nginx as a proxy in front of #FedBOX in favour of Caddy 2.0 that was just released. The reason for this move was to enable HTTP/2.0 on FedBOX with an ultimate goal of allowing request pipelining when loading collections from littr.me. I'm still to add support for this in littr.me though.

#ActivityPub #updates #may #new-version
