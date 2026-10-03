-- Comic roots/files predate comic variant support and are all variant=''
-- today — handleAddRootFolder used to force every non-manga root to ''
-- outright, so no comic root could ever get a real variant tag through the
-- normal API. Now that comics can opt into variant tracking the same way
-- manga does, give existing comic roots/files the same one-time backfill
-- migration 014 gave manga, with the inverse default: color is the standard
-- form for comics (unlike manga, where mono is standard), so a Noir/B&W
-- root is the deliberate exception the user adds explicitly, not the other
-- way around.

UPDATE root_folders SET variant = 'color' WHERE media_type = 'comic' AND variant = '';

UPDATE book_files
   SET variant = COALESCE((SELECT variant FROM root_folders WHERE root_folders.id = book_files.root_folder_id), '')
 WHERE variant = '' AND media_type = 'comic';
