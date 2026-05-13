# Darktide-Servo-ModQuisitor-2
This is a program for sorting Darktide mods.
<img src="https://private-user-images.githubusercontent.com/21146468/591441814-9ffdba3a-b93d-49c7-b823-125e16a06dbd.png?jwt=eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJpc3MiOiJnaXRodWIuY29tIiwiYXVkIjoicmF3LmdpdGh1YnVzZXJjb250ZW50LmNvbSIsImtleSI6ImtleTUiLCJleHAiOjE3Nzg2MzY3NTQsIm5iZiI6MTc3ODYzNjQ1NCwicGF0aCI6Ii8yMTE0NjQ2OC81OTE0NDE4MTQtOWZmZGJhM2EtYjkzZC00OWM3LWI4MjMtMTI1ZTE2YTA2ZGJkLnBuZz9YLUFtei1BbGdvcml0aG09QVdTNC1ITUFDLVNIQTI1NiZYLUFtei1DcmVkZW50aWFsPUFLSUFWQ09EWUxTQTUzUFFLNFpBJTJGMjAyNjA1MTMlMkZ1cy1lYXN0LTElMkZzMyUyRmF3czRfcmVxdWVzdCZYLUFtei1EYXRlPTIwMjYwNTEzVDAxNDA1NFomWC1BbXotRXhwaXJlcz0zMDAmWC1BbXotU2lnbmF0dXJlPTJmNGNjZGJhMmIzZTM5ZDZjMjdkMjgyYjU0NTYwYjY2MmY5ZjI4OTc3ZWFiZGQ0MmYxMjY4NzZiMmQ1OTZlMjYmWC1BbXotU2lnbmVkSGVhZGVycz1ob3N0JnJlc3BvbnNlLWNvbnRlbnQtdHlwZT1pbWFnZSUyRnBuZyJ9.JZSmiu1P2v4PvI8rdlI2X-uh5svNq2rDjCP5AAkV0x8">

## Features:
- Drag and drop mod installation directly onto the program window.
- One-click automatic problem detection and fixes.
- Manual list customization is available.
- Group mod reordering is available.
- Disabling the main checkbox disables the mod in the "--Mod" list.

## ModQuizitor scans for mod-related issues:
- **Outdated** mods,
- **Malformed** folders _(ModName-222-33-4-5-666)_,
- **Empty** folders,
- Mod **conflicts**,
- Mod **dependencies**.
It fixes these issues if possible or suggests solutions.

## TODO:
- [ ] Finish the mod lists _(**~11%** of all mods checked and added)_.
- [ ] Finish the Light Theme.
- [ ] Linux support (the program can be compiled already, but I can’t check it yet).

#### Two mod management modes are available:
1. Manual, using "Mod Management,"
2. Automatic, using "Check and Auto-Sorting." In automatic mode, everything happens almost instantly.

## Installation:
1. <a href="https://github.com/xsSplater/Darktide-Servo-Modquisitor-2/releases">Download the program</a>.
2. Install it in the MODS folder.
3. Download the list files <a href="https://github.com/xsSplater/Darktide-Servo-Modquisitor-2/tree/main/SortingRules_and_ModDatabase">mandatory_obsolete_incompatible_dependencies.json</a> and <a href="https://github.com/xsSplater/Darktide-Servo-Modquisitor-2/tree/main/SortingRules_and_ModDatabase">mod_database.json</a> (you can do this directly from the program).
- If downloading manually, place them in the MODS folder.
4. Download custom lists (<a href="https://www.nexusmods.com/warhammer40kdarktide/mods/139?tab=files">[english_sort_order.txt]</a> or <a href="https://www.nexusmods.com/warhammer40kdarktide/mods/139?tab=files">[russian_sort_order.txt]</a>), if needed.
- Unzip to the MODS folder.
5. Run Auto Sort.
- Check the file if necessary (it will open automatically).
6. Now you can launch the game.

## Mod Management:
1. Open the program.
2. Click the "Mod Management" button.
3. Manage mods using the buttons and checkboxes: move them and arrange them as you like.
4. Click "Save List."
5. Now you can launch the game.

<img src="https://private-user-images.githubusercontent.com/21146468/591457183-5cf9b8cb-1aee-41a2-812e-a57c52bb62ce.png?jwt=eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJpc3MiOiJnaXRodWIuY29tIiwiYXVkIjoicmF3LmdpdGh1YnVzZXJjb250ZW50LmNvbSIsImtleSI6ImtleTUiLCJleHAiOjE3Nzg2Mzc0ODksIm5iZiI6MTc3ODYzNzE4OSwicGF0aCI6Ii8yMTE0NjQ2OC81OTE0NTcxODMtNWNmOWI4Y2ItMWFlZS00MWEyLTgxMmUtYTU3YzUyYmI2MmNlLnBuZz9YLUFtei1BbGdvcml0aG09QVdTNC1ITUFDLVNIQTI1NiZYLUFtei1DcmVkZW50aWFsPUFLSUFWQ09EWUxTQTUzUFFLNFpBJTJGMjAyNjA1MTMlMkZ1cy1lYXN0LTElMkZzMyUyRmF3czRfcmVxdWVzdCZYLUFtei1EYXRlPTIwMjYwNTEzVDAxNTMwOVomWC1BbXotRXhwaXJlcz0zMDAmWC1BbXotU2lnbmF0dXJlPTA2OThiZDA2NDUxZmIyZGMwYWVkZGRkZjg3MGVmZmIxYjU2Y2VlZWFjYmZhNzU3ZDBjMDljZmJmMzBmMjVlZmQmWC1BbXotU2lnbmVkSGVhZGVycz1ob3N0JnJlc3BvbnNlLWNvbnRlbnQtdHlwZT1pbWFnZSUyRnBuZyJ9.WFx87sHblfRMH-RU3kVcDgGbQaWxwf4yBqGWVzf60R8">

