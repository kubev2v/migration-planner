#!/bin/bash

# Ensure we're using Bash (not sh)
if [ -z "$BASH_VERSION" ]; then
    echo "This script requires Bash. Please run it using 'bash convert_template.sh ...'"
    exit 1
fi

# Check if required arguments are provided
if [ "$#" -lt 2 ] ; then
    echo "Usage: $0 <TEMPLATE_FILE> <OUTPUT_FILE> [PARAMETERS_FILE]"
    exit 1
fi

TEMPLATE_FILE="$1"
OUTPUT_FILE="$2"

# Check if yq is installed
if ! command -v yq &> /dev/null; then
    echo "Error: 'yq' is required but not installed. Install it via 'sudo apt install yq' or 'brew install yq'."
    exit 1
fi

declare -a param_keys
declare -a param_values

# **Step 1: Extract parameters from the template file**
echo "Extracting parameters from template..."
while IFS=':' read -r name default; do
    name=$(echo "$name" | xargs)       # Trim whitespace
    default=$(echo "$default" | xargs) # Trim whitespace
    if [[ -n "$name" ]]; then
        param_keys+=("$name")
        param_values+=("${default:-}") # Use default value if not set
    fi
done < <(yq eval '.parameters[] | .name + ":" + (.value // "")' "$TEMPLATE_FILE")

# **Step 2: Override with values from the parameters file (if provided)**
if [[ -n "$3" && -f "$3" ]]; then
    PARAMETERS_FILE="$3"
    shift  # Move past the parameters file argument
    echo "Loading parameters from $PARAMETERS_FILE..."
    while IFS='=' read -r key value; do
        key=$(echo "$key" | xargs)     # Trim whitespace
        value=$(echo "$value" | xargs) # Trim whitespace
        if [[ -n "$key" ]]; then
            for i in "${!param_keys[@]}"; do
                if [[ "${param_keys[$i]}" == "$key" ]]; then
                    param_values[$i]="$value"
                    break
                fi
            done
        fi
    done < "$PARAMETERS_FILE"
fi

# **Step 3: Override with values from command-line arguments (-p PARAM_KEY=PARAM_VAL)**
while [[ "$#" -gt 0 ]]; do
    case "$1" in
        -p)
            if [[ "$2" =~ ^([^=]+)=(.*)$ ]]; then
                key="${BASH_REMATCH[1]}"
                value="${BASH_REMATCH[2]}"
                for i in "${!param_keys[@]}"; do
                    if [[ "${param_keys[$i]}" == "$key" ]]; then
                        param_values[$i]="$value"  # Override existing value
                        break
                    fi
                done
                shift 2  # Move past -p and its argument
            else
                echo "Error: Invalid parameter format. Use -p PARAM_KEY=PARAM_VAL"
                exit 1
            fi
            ;;
        *)
            echo "Warning: Unrecognized argument '$1', skipping..."
            shift
            ;;
    esac
done

echo "Final parameter values:"
for i in "${!param_keys[@]}"; do
    echo "${param_keys[$i]}=${param_values[$i]}"
done

# Extract objects and format them with '---'
yq eval '.objects[] | "---\n" + to_yaml' "$TEMPLATE_FILE" > "$OUTPUT_FILE.tmp"

# Replace parameters in the output file
cp "$OUTPUT_FILE.tmp" "$OUTPUT_FILE"

for i in "${!param_keys[@]}"; do
    key="${param_keys[$i]}"
    value="${param_values[$i]}"
    value=${value//\//\\/}  # Escape slashes in parameter values

    key_regex="\\\${$key}"      # Matches ${VAR}
    key_regex_double="\\\${{${key}}}"  # Matches ${{VAR}}

    if [[ "$OSTYPE" == "darwin"* ]]; then
        sed -i '' "s|$key_regex|$value|g" "$OUTPUT_FILE"
        sed -i '' "s|$key_regex_double|$value|g" "$OUTPUT_FILE"
    else
        sed -i "s|$key_regex|$value|g" "$OUTPUT_FILE"
        sed -i "s|$key_regex_double|$value|g" "$OUTPUT_FILE"
    fi
done

# Cleanup temp file
rm -f "$OUTPUT_FILE.tmp"

echo "Template successfully converted to manifest: $OUTPUT_FILE"