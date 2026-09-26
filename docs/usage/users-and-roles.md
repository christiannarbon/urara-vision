# Users and roles

Everyone signs in with a username and password. What you can do depends on
your role.

## The three roles

| Role | Can |
|---|---|
| **Viewer** | Read every project and version, compare versions, and use chat |
| **Creator** | Everything a viewer can, plus import new projects and new versions |
| **Admin** | Everything a creator can, plus delete projects and versions, switch chat on or off, and manage users |

Buttons you cannot use are hidden. The server refuses the action too, so a
hidden button is never the only protection. If an admin changes your role while
you are signed in, the next refused action says "You are not allowed to do
that" and the page updates to match your new role.

## Managing users (admins)

Open the menu under your name in the top right and choose **Users**.

- **New user**: choose a username, an optional display name, a role and a
  password of at least 12 characters. The username cannot be changed later.
- **Change a role**: pick a new role in the user's row. It takes effect on
  their next request.
- **Reset password**: sets a new password and signs the user out everywhere.
- **Delete**: removes the user after a confirmation. You cannot delete
  yourself.

**There is always at least one admin.** Demoting or deleting the last admin is
refused with "at least one admin must remain", and the role you picked snaps
back.

## Conversations are private

A chat conversation belongs to the person who started it. Nobody else sees it
in their list or can open it, admins included. Conversations started before
roles existed have no owner and are no longer listed.

## When a user is deleted

- Their sessions end at once.
- Their conversations are deleted.
- Projects and versions they imported stay.
