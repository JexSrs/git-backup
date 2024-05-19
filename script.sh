#!/bin/bash
set -e

# Environment variables
GITHUB_TOKEN=""
GITLAB_URL=""
GITLAB_TOKEN=""
STORAGE_URL=""

# Argument variables
_GITHUB_USERNAME=''
_GITLAB_ID=''
_INCLUDE_ONLY=''
_EXCLUDE=''
_SKIP=''
_EXLUDE_WIKI_FOR=''
_INCLUDE_WIKI_FOR=''
_EXLUDE_RELEASES_FOR=''
_INCLUDE_RELEASES_FOR=''
_EXLUDE_ASSETS_FOR=''
_INCLUDE_ASSETS_FOR=''
_ASSETS_THRESHOLD='5'
_ASSETS_MAX_SIZE='1GB'


# Extract options and their arguments into variables
while [[ $# -gt 0 ]]; do
  case "$1" in
        --github-username) _GITHUB_USERNAME="$2"; shift 2 ;;
        --gitlab-id) _GITLAB_ID="$2"; shift 2 ;;
        --include-only) _INCLUDE_ONLY="$2"; shift 2 ;;
        --exclude) _EXCLUDE="$2"; shift 2 ;;
        --skip) _SKIP="$2"; shift 2 ;;
        --exclude-wiki-for) _EXLUDE_WIKI_FOR="$2"; shift 2 ;;
        --include-wiki-only-for) _INCLUDE_WIKI_FOR="$2"; shift 2 ;;
        --exclude-releases-for) _EXLUDE_RELEASES_FOR="$2"; shift 2 ;;
        --inlclude-releases-only-for) _INCLUDE_RELEASES_FOR="$2"; shift 2 ;;
        --exclude-assets-for) _EXLUDE_ASSETS_FOR="$2"; shift 2 ;;
        --include-assets-only-for) _INCLUDE_ASSETS_FOR="$2"; shift 2 ;;
        --delete-release-assets-threshold) _ASSETS_THRESHOLD="$2"; shift 2 ;;
        --asset-max-size) _ASSETS_MAX_SIZE="$2"; shift 2 ;;
        --) shift; break ;;
        *) echo "Unexpected option: $1"; exit 1 ;;
    esac
done

echo "GitHub username: $_GITHUB_USERNAME"
echo "GitLab group ID: $_GITLAB_ID"
echo "Include only: $_INCLUDE_ONLY"
echo "Exclude: $_EXCLUDE"
echo "Skip: $_SKIP"

function convert_to_bytes() {
    size_string=$1

    # Extract the numeric part of the size string
    size=$(echo $size_string | sed 's/[^0-9]//g')

    # Extract the unit part of the size string
    unit=$(echo $size_string | sed 's/[0-9]//g')

    # Convert the size to bytes based on the unit
    case $unit in
        "KB") bytes=$(($size * 1024));;
        "MB") bytes=$(($size * 1024 * 1024));;
        "GB") bytes=$(($size * 1024 * 1024 * 1024));;
        *) bytes=$size;; # Assume bytes if no unit is specified
    esac

    echo $bytes
}

# This function sets the original github url to a CI/CD variable in the GitLab repo.
#
# Usage: set_github_url $project_id $repo_url
# arg1 - The GitLab project id
# arg2 - The GitHub repository url
#
# Returns 0 if the variable was set successfully, 1 otherwise.
function set_github_url() {
    project_id=$1
    repo_url=$2

    variable_response_body=$(mktemp)
    http_status=$(curl --silent --output "$variable_response_body" --write-out "%{http_code}" \
            --request POST --header "PRIVATE-TOKEN: $GITLAB_TOKEN" \
            --form "key=github_url" --form "value=$repo_url" \
            "$GITLAB_URL/api/v4/projects/$project_id/variables")

    # Check if the response status code is not in the 2xx range
    if [[ $http_status -lt 200 || $http_status -ge 300 ]]; then
        echo "Failed to set variable. HTTP status code: $http_status"
        echo "Response body:"
        cat "$variable_response_body"
        rm "$variable_response_body"
        return 1
    else
        rm "$variable_response_body"
        return 0
    fi
}

# This function locks the execution until the import of a GitLab project is finished.
#
# Usage: lock_until_import $project_id
# arg1 - The GitLab project id
#
# This function outputs the import progress directly to the standard output.
# Returns 0 if the import was successful, 1 otherwise.
function lock_until_import() {
    project_id=$1

    while : ; do
        # Query the GitLab API for project details
        project_details=$(curl -s --header "PRIVATE-TOKEN: $GITLAB_TOKEN" "$GITLAB_URL/api/v4/projects/$project_id")
        import_status=$(echo "$project_details" | jq -r '.import_status')
        # Check if the import_status is 'finished'
        if [ "$import_status" = "finished" ]; then
            import_finished=true
            echo "  - Import finished successfully"
            break
        elif [ "$import_status" = "failed" ]; then
            echo "  - Current import status: $import_status"
            return 1
        else
            echo "  - Current import status: $import_status"
            sleep 5
        fi
    done

    return 0
}

# This function downloads an asset from a URL.
#
# Usage: download_asset $asset_url $asset_name
# arg1 - The asset URL
# arg2 - The asset name
#
# This function outputs any errors directly to the standard output.
# Returns 0 if the variable was set successfully, 1 otherwise.
function download_asset() {
    asset_url=$1
    asset_name=$2

    variable_response_body=$(mktemp)
    http_status=$(curl --silent --write-out "%{http_code}" \
                    -L $asset_url -o "$asset_name")

    # Check if the response status code is not in the 2xx range
    if [[ $http_status -lt 200 || $http_status -ge 300 ]]; then
        echo "Failed to set variable. HTTP status code: $http_status"
        echo "Response body:"
        cat "$variable_response_body"
        rm "$variable_response_body"
        return 1
    else
        rm "$variable_response_body"
        return 0
    fi
}

# This function uploads an asset to Storage and links it to a GitLab release.
#
# Usage: create_asset $project_id $tag_name $asset_name $asset_url
# arg1 - The GitLab project id
# arg2 - The release tag name
# arg3 - The asset name
# arg4 - The asset URL
#
# This function outputs any errors directly to the standard output.
# Returns 0 if the variable was set successfully, 1 otherwise.
function create_asset() {
    project_id=$1
    tag_name=$2
    asset_name=$3
    asset_url=$4

    # Encoe tag name
    encoded_tag_name=$(printf '%s' "$tag_name" | jq -sRr @uri)

    variable_response_body=$(mktemp)
    # Link the asset to the release
    http_status=$(curl --silent --output "$variable_response_body" --write-out "%{http_code}" \
                        --request POST --header "PRIVATE-TOKEN: $GITLAB_TOKEN" \
                        "$GITLAB_URL/api/v4/projects/$project_id/releases/$encoded_tag_name/assets/links?name=$asset_name&url=${asset_url}")

    # Check if the response status code is not in the 2xx range
    if [[ $http_status -lt 200 || $http_status -ge 300 ]]; then
        echo "Failed to set variable. HTTP status code: $http_status"
        echo "Response body:"
        cat "$variable_response_body"
        rm "$variable_response_body"
        return 1
    else
        rm "$variable_response_body"
        return 0
    fi
}

# This function deletes an asset from storage.
#
# Usage: delete_assets $project_id $tag_name
# arg1 - The GitLab project id
# arg2 - The release tag name
#
# Returns 0
function delete_assets() {
    project_id=$1
    tag_name=$2

    # Delete from storage
    curl -X DELETE "${STORAGE_URL}/gitlab/projects/prj_${project_id}/tag_${tag_name//\//-}"
    return 0
}

function sync_repo() {
    count=$1
    repo=$2
    sync_wiki=$3
    sync_releases=$4
    sync_assets=$5

    jq_repo() {
        echo ${repo} | base64 --decode | jq -r "${1}"
    }
    
    repo_name=$(jq_repo '.name')
    repo_url=$(jq_repo '.clone_url')
    repo_description=$(jq_repo '.description')

    echo ""
    echo "$count. Evaluating GitHub repository '$repo_name'"

    # Check if repo exists in GitLab group
    exists=$(curl -s --header "Private-Token: $GITLAB_TOKEN" "$GITLAB_URL/api/v4/groups/$_GITLAB_ID/projects?search=$repo_name")    
    lowercase_repo_name=$(echo "$repo_name" | tr '[:upper:]' '[:lower:]')
    matching_project=$(echo "$exists" | jq --arg repo_name "$lowercase_repo_name" 'map(.name | ascii_downcase) | index($repo_name)')

    if [ "$matching_project" == "null" ]; then
        mkdir "$repo_name"
        cd "$repo_name"

        echo "- Importing new repository in GitLab..."
        # Create a project
        project=$(curl -s --request POST --header "Private-Token: $GITLAB_TOKEN" \
            --data "name=${repo_name#.}" \
            --data "namespace_id=$_GITLAB_ID" \
            --data "import_url=$repo_url" \
            --data-urlencode "description=$repo_description" \
            "$GITLAB_URL/api/v4/projects")

        project_id=$(echo "$project" | jq -r '.id')
        echo "- GitLab project id: $project_id"

        # Store original GitHub URL as a custom attribute
        echo "- Create 'github_url' attribute with value: $repo_url"
        set_github_url $project_id $repo_url

        # Check repo status
        echo "- Waiting for repository import to finish..."
        lock_until_import $project_id

        # Unprotect branches
        branches=$(curl -s --header "Private-Token: $GITLAB_TOKEN" "$GITLAB_URL/api/v4/projects/$project_id/protected_branches")
        branch_names=$(echo "$branches" | jq -r '.[].name')
        echo "- Found ${#branch_names[@]} branches"
        echo "  - Unprotecting branches..."
        for branch in $branch_names; do
            echo "    - Unprotecting $branch..."
            encoded_branch=$(jq -nr --arg branch "$branch" '$branch|@uri')
            curl -s --request DELETE --header "Private-Token: $GITLAB_TOKEN" \
                "$GITLAB_URL/api/v4/projects/$project_id/protected_branches/$encoded_branch" 2>&1 | sed 's/^/"      /'
        done
    else
        echo "- Repository $repo_name already exists in GitLab group $_GITLAB_ID"
        project=$(echo "$exists" | jq ".[$matching_project]")
        project_id=$(echo "$project" | jq -r '.id')
        gitlab_repo_http_url=$(echo "$project" | jq -r '.http_url_to_repo')
        echo "- GitLab project id: $project_id"

        # Sync GitHub repository to GitLab
        echo "- Clone GitHub repository"
        git clone "$repo_url" "$repo_name" 2>&1 | sed 's/^/    /'
        cd "$repo_name"

        gitlab_remote=$(echo "$gitlab_repo_http_url" | sed -E "s|(https?)://|\1://oauth2:$GITLAB_TOKEN@|")
        git remote add gitlab "$gitlab_remote"

        echo "- Pushing branches to GitLab..."
        branches=$(git branch --list | sed 's/^\*//g' | sed 's/^[ \t]*//')
        echo "  - Found ${#branches[@]} branches"
        for branch in "${branches[@]}"; do
            echo "  - Pushing $branch..."
            git push gitlab "$branch" --force 2>&1 | sed 's/^/      /'
            # Check if the push was successful
            if [ $? -ne 0 ]; then
                return 1
            fi
        done

        echo "- Pushing tags to GitLab..."
        git push --tags gitlab 2>&1 | sed 's/^/    /'
        # Check if the push was successful
        if [ $? -ne 0 ]; then
            return 1
        fi
    fi

    # Fetch GitHub releases
    if [ "$sync_releases" == true ]; then
        echo "- Fetching GitHub releases..."
        github_releases=$(curl -s -H "Authorization: Bearer $GITHUB_TOKEN" "https://api.github.com/repos/$_GITHUB_USERNAME/$repo_name/releases?per_page=10")
        github_releases=$(echo $github_releases | jq '. | reverse') # reversed so the oldest are first
        echo "  - Found $(echo "$github_releases" | jq '. | length') releases"

        for release in $(echo "$github_releases" | jq -r '.[] | @base64'); do
            jq_release() {
                echo ${release} | base64 --decode | jq -r "${1}"
            }

            tag_name=$(jq_release '.tag_name')
            release_name=$(jq_release '.name')
            release_description=$(jq_release '.body')
            release_assets=$(jq_release '.assets | .[] | @base64')
            release_created_at=$(jq_release '.created_at')

            echo "  - Evaluating release $tag_name..."

            gitlab_release=$(curl -s --header "Private-Token: $GITLAB_TOKEN" "$GITLAB_URL/api/v4/projects/$project_id/releases/$tag_name")
            if [[ "$gitlab_release" == *"404 Not Found"* ]]; then
                echo "    - Release $tag_name does not exist, creating..."

                gitlab_release=$(curl -s --request POST --header "Private-Token: $GITLAB_TOKEN" \
                    --data-urlencode "name=$release_name" \
                    --data-urlencode "tag_name=$tag_name" \
                    --data-urlencode "description=$release_description" \
                    --data-urlencode "released_at=$release_created_at" \
                    "$GITLAB_URL/api/v4/projects/$project_id/releases")
                
                # Sync assets
                assets_count=$(echo ${release} | base64 --decode | jq '.assets | length')
                echo "    - Found $assets_count assets"
                for asset in $release_assets; do
                    jq_asset() {
                        echo ${asset} | base64 --decode | jq -r ${1}
                    }

                    asset_name=$(jq_asset '.name')
                    asset_url=$(jq_asset '.browser_download_url')

                    if [[ "$sync_assets" == true ]]; then
                        # Download the asset
                        echo "    - Downloading asset: $asset_name"
                        download_asset $asset_url $asset_name

                        # Check if the asset exceeds the maximum size
                        if [ "$_ASSETS_MAX_SIZE" != "none" ]; then
                            size=$(stat -f %z $asset_name)
                            echo "      - Size: $size"
                            bytes=$(convert_to_bytes $_ASSETS_MAX_SIZE)

                            if [ $size -gt $bytes ]; then
                                echo "      - Asset $asset_name exceeds the maximum size of $_ASSETS_MAX_SIZE"
                                rm -f "./$asset_name"
                            else
                                # Upload the asset to dufs
                                echo "      - Uploading asset to storage and link to GitLab..."
                                asset_url="${STORAGE_URL}/gitlab/projects/prj_${project_id}/tag_${tag_name//\//-}/${asset_name//\//-}"
                                curl -T "./$asset_name" "$asset_url"
                            fi
                        fi
                    fi
                    
                    # Link to GitLab (either with dufs or GitHub URL)
                    create_asset $project_id $tag_name $asset_name $asset_url
                    rm -f "./$asset_name"
                    echo "      - Done"
                done
            else
                echo "    - Release $tag_name already exists in GitLab"
            fi
        done

        # Delete older releases' assets
        if [ "$_ASSETS_THRESHOLD" != "none" ]; then
            echo "- Checking older releases' assets to delete from storage..."
            gitlab_releases=$(curl -s --header "Private-Token: $GITLAB_TOKEN" "$GITLAB_URL/api/v4/projects/$project_id/releases") # Fetch latest releases from GitLab
            total_gitlab_releases=$(echo "$gitlab_releases" | jq '. | length')
            gitlab_releases_to_process=$((total_gitlab_releases - _ASSETS_THRESHOLD))

            # Check if there are releases to process
            if [ $gitlab_releases_to_process -gt 0 ]; then
                echo "  - Processing $gitlab_releases_to_process releases for asset deletion"
                for release_index in $(seq 0 $((gitlab_releases_to_process - 1))); do
                    release=$(echo "$gitlab_releases" | jq ".[$release_index]")
                    tag_name=$(echo "$release" | jq -r '.tag_name')

                    # Delete assets from storage
                    echo "  - Deleting assets for release $tag_name from storage"
                    delete_assets $project_id $tag_name
                done
            fi
        fi
    fi

    cd ..
    rm -rf "./$repo_name"

    # Fetch GitHub wiki
    if [ "$sync_wiki" == true ]; then
        echo "- Checking for GitHub wiki..."
        github_wiki_url="https://$GITHUB_TOKEN:x-oauth-basic@github.com/$_GITHUB_USERNAME/$repo_name.wiki.git"

        project_path=$(echo "$project" | jq -r '.path_with_namespace')
        gitlab_wiki_url="$GITLAB_URL/${project_path}.wiki.git"
        gitlab_wiki_url_with_token=$(echo "$gitlab_wiki_url" | sed -E "s|(https?)://|\1://oauth2:$GITLAB_TOKEN@|")

        if git ls-remote "$github_wiki_url" &> /dev/null; then
            echo "  - Found GitHub Wiki, syncing..."
            git clone "$github_wiki_url" "${repo_name}_wiki" 2>&1 | sed 's/^/      /'
            cd "${repo_name}_wiki"

            # Push to GitLab Wiki
            git remote add gitlab "$gitlab_wiki_url_with_token"

            echo "  - Pushing branches to GitLab..."
            branches=$(git branch --list | sed 's/^\*//g' | sed 's/^[ \t]*//')
            echo "    - Found ${#branches[@]} branches"
            for branch in "${branches[@]}"; do
                echo "  - Pushing $branch..."
                git push gitlab "$branch" --force 2>&1 | sed 's/^/        /'
                # Check if the push was successful
                if [ $? -ne 0 ]; then
                    return 1
                fi
            done

            cd ..
            rm -rf "./${repo_name}_wiki"
        fi
    fi

    return 0
}

function sync_user() {
    IFS=',' read -r -a repo_exlude <<< "$_EXCLUDE"
    IFS=',' read -r -a repo_include_only <<< "$_INCLUDE_ONLY"
    IFS=',' read -r -a repo_wiki_exlude <<< "$_EXLUDE_WIKI_FOR"
    IFS=',' read -r -a repo_wiki_include_only <<< "$_INCLUDE_WIKI_FOR"
    IFS=',' read -r -a repo_releases_exlude <<< "$_EXLUDE_RELEASES_FOR"
    IFS=',' read -r -a repo_releases_include_only <<< "$_EXLUDE_RELEASES_FOR"
    IFS=',' read -r -a repo_assets_exlude <<< "$_EXLUDE_ASSETS_FOR"
    IFS=',' read -r -a repo_assets_include_only <<< "$_INCLUDE_ASSETS_FOR"
    
    sync_dir="./tmp_$_GITHUB_USERNAME"

    rm -rf $sync_dir
    mkdir -p $sync_dir
    cd $sync_dir

    page=1
    count=1
    while : ; do
        # Get repositories from the GitHub API
        response=$(curl -s -H "Authorization: Bearer $GITHUB_TOKEN" "https://api.github.com/users/$_GITHUB_USERNAME/repos?per_page=100&page=$page")
        if [ "$(echo "$response" | jq '. | length')" -eq 0 ]; then
            break
        fi

        # Loop through each repository
        for repo in $(echo "$response" | jq -r '.[] | @base64'); do
            repo_name=$(echo "$repo" | base64 --decode | jq -r '.name')

            # Skip the repository if its name is .github
            if [ "$repo_name" == ".github" ]; then
                echo "Skipping repository $repo_name"
                continue
            fi

            # Check if _SKIP is not empty and if count is less than or equal to _SKIP
            if [ -n "$_SKIP" ] && [ $count -le $_SKIP ]; then
                echo "Skipping repository $repo_name as it's in the list of skipped repositories by count"
                ((count++))
                continue
            fi

            # Check if repo_include_only is not empty and if repo_name is not in repo_include_only
            if [ ${#repo_include_only[@]} -ne 0 ] && ! [[ " ${repo_include_only[@]} " =~ " ${repo_name} " ]]; then
                echo "Skipping repository $repo_name as it's not in the list of specific repositories."
                continue
            fi
            
            # Check if repo_exlude is not empty and if repo_name is in repo_exlude
            if [ ${#repo_exlude[@]} -ne 0 ] && [[ " ${repo_exlude[@]} " =~ " ${repo_name} " ]]; then
                echo "Skipping repository $repo_name as it's in the list of excluded repositories."
                continue
            fi

            sync_wiki=true
            sync_releases=true
            sync_assets=true

            # Check if repo_wiki_include_only is not empty and if repo_name is not in repo_wiki_include_only
            # Check if repo_wiki_exlude is not empty and if repo_name is in repo_wiki_exlude
            if { [ ${#repo_wiki_include_only[@]} -ne 0 ] && ! [[ " ${repo_wiki_include_only[@]} " =~ " ${repo_name} " ]]; } ||
                { [ ${#repo_wiki_exclude[@]} -ne 0 ] && [[ " ${repo_wiki_exclude[@]} " =~ " ${repo_name} " ]]; }; then
                sync_wiki=false
            fi

            # Check if repo_releases_include_only is not empty and if repo_name is not in repo_releases_include_only
            # Check if repo_releases_exlude is not empty and if repo_name is in repo_releases_exlude
            if { [ ${#repo_releases_include_only[@]} -ne 0 ] && ! [[ " ${repo_releases_include_only[@]} " =~ " ${repo_name} " ]]; } ||
                { [ ${#repo_releases_exlude[@]} -ne 0 ] && [[ " ${repo_releases_exlude[@]} " =~ " ${repo_name} " ]]; }; then
                sync_releases=false
            fi

            # Check if repo_assets_include_only is not empty and if repo_name is not in repo_assets_include_only
            # Check if repo_assets_exlude is not empty and if repo_name is in repo_assets_exlude
            if { [ ${#repo_assets_include_only[@]} -ne 0 ] && ! [[ " ${repo_assets_include_only[@]} " =~ " ${repo_name} " ]]; } ||
                { [ ${#repo_assets_exlude[@]} -ne 0 ] && [[ " ${repo_assets_exlude[@]} " =~ " ${repo_name} " ]]; }; then
                sync_assets=false
            fi

            sync_repo $count $repo $sync_wiki $sync_releases $sync_assets
            ((count++))
            sleep 2
        done

        # Move to the next page
        ((page++))
    done

    cd ..
    rm -rf $sync_dir
}

sync_user;
exit 0;