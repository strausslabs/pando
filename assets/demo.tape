# Render with: bash assets/demo-setup.sh && vhs assets/demo.tape
# Output: assets/demo.gif

Output assets/demo.gif

Set Shell "bash"
Set FontSize 18
Set Width 1100
Set Height 720
Set Padding 24
Set Theme "Catppuccin Mocha"
Set TypingSpeed 45ms
Set PlaybackSpeed 1.0

Set Margin 20
Set MarginFill "#1a1b26"
Set BorderRadius 12

Hide
Type "export PATH=/tmp/pando-demo-bin:$PATH; cd /tmp/pando-grove; clear" Enter
Sleep 500ms
Show

Type "# one repo, two branches checked out as worktrees" Sleep 600ms Enter
Type "git worktree list" Sleep 400ms Enter
Sleep 2s

Type "# describe the dev environment once, in pando.star" Sleep 600ms Enter
Type "cat pando.star" Sleep 400ms Enter
Sleep 3s

Type "# bring both branches up — pando starts the daemon + dashboard for you" Sleep 600ms Enter
Type "pando up && pando up -w feature-checkout" Sleep 400ms Enter
Sleep 12s

Type "clear" Enter
Type "# both branches live — each api got its own $PORT_api, no collisions" Sleep 800ms Enter
Type "pando status" Sleep 400ms Enter
Sleep 4s

Type "# $PORT_api expands to that port everywhere it's referenced —" Sleep 600ms Enter
Type "# here the client on each branch hits its own api" Sleep 600ms Enter
Type "pando logs client -w master --tail 2" Sleep 400ms Enter
Sleep 2s
Type "pando logs client -w feature-checkout --tail 2" Sleep 400ms Enter
Sleep 3s

Type "# regex search across a resource's logs" Sleep 600ms Enter
Type "pando logs client -w master --grep '200|ok' --tail 2" Sleep 400ms Enter
Sleep 3s

Type "# dashboard is live the whole time at the repo-derived port" Sleep 800ms Enter
Sleep 2s
