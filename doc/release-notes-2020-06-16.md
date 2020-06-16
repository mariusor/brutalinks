## The "moderation is coming" release 

The past month I've been working on moderation. (Mostly.)

This represents the **last big hurdle**<sup>[1]</sup> before I can start actively pursuing server to server interactions.

### Moderation

The first steps (in my eyes) for valid cross-instance moderation is allowing users to silence other users, 
threads, or full instances.

In this release I added the capability for users to block other users. 
It is not fully up [to spec](https://www.w3.org/TR/activitypub/#block-activity-outbox), as I'm not entirely
sure how to "prevent the blocked user from interacting with any object posted by the actor". 

The way I interpret this statement is that users shouldn't be allowed to see objects created by users that block them, 
which is not really feasible in my opinion, since the blocked user can access those same objects if they're not using
authentication when retrieving them.

In littr.me the blocking mechanism requires providing a text "reason" before being sent to the server. This would allow
a user to remember why they blocked someone else, and also, as a general warning for others to see in the moderation log.

Speaking of the moderation log, currently, the `Block` activities generated are not publicly disseminated, 
and as such are not visible to other users. This proves to be an issue as users can't see the activities 
because they're private to the account that created them. I need to find a suitable method of allowing public access 
to an anonymised version of the activity. 

### Privately stored activities

Most of the time, however I spent working on the go-ap packages, because this feature served 
as the start point for a large detour in the storage layer of #fedbox, the #activitypub service that 
constitutes the backend of littr.me, which resulted in the capability of storing "private" activities in actors' 
outboxes. 

This is a very important step forward in removing the need for a custom "activities" collection for #fedbox.

### Next steps

Next I'm planning to add `Flag` activity support, and maybe `Ignore`, even though I'm not entirely sure what differences
there should be in how the server treats one in comparison to a `Block`.
___

[1] From the "famous last words" department.
