#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GRADLE_FILE="${1:-${ROOT}/apps/desktop/src-tauri/gen/android/app/build.gradle.kts}"

if [[ ! -f "${GRADLE_FILE}" ]]; then
  echo "missing generated Android Gradle file: ${GRADLE_FILE}" >&2
  exit 1
fi

ruby - "${GRADLE_FILE}" <<'RUBY'
path = ARGV.fetch(0)
text = File.read(path)

unless text.include?("linettaKeystorePropertiesFile")
  insert = <<~'KTS'
    val linettaKeystorePropertiesFile = rootProject.file("keystore.properties")
    val linettaKeystoreProperties = Properties().apply {
        if (linettaKeystorePropertiesFile.exists()) {
            linettaKeystorePropertiesFile.inputStream().use { load(it) }
        }
    }

  KTS
  text = text.sub(/\nandroid \{\n/, "\n#{insert}android {\n")
end

unless text.include?("linettaKeystoreProperties")
  abort("failed to add Linetta keystore properties block")
end

unless text.include?("create(\"release\")")
  signing = <<~'KTS'
        signingConfigs {
            if (linettaKeystorePropertiesFile.exists()) {
                create("release") {
                    keyAlias = linettaKeystoreProperties["keyAlias"] as String
                    keyPassword = linettaKeystoreProperties["password"] as String
                    storeFile = file(linettaKeystoreProperties["storeFile"] as String)
                    storePassword = linettaKeystoreProperties["password"] as String
                }
            }
        }
        buildTypes {
  KTS
  text = text.sub(/    buildTypes \{\n/, signing)
end

release_hook = <<~'KTS'
        getByName("release") {
            if (linettaKeystorePropertiesFile.exists()) {
                signingConfig = signingConfigs.getByName("release")
            }
KTS
unless text.include?("signingConfigs.getByName(\"release\")")
  text = text.sub(/        getByName\("release"\) \{\n/, release_hook)
end

File.write(path, text)
RUBY

grep -Fq 'rootProject.file("keystore.properties")' "${GRADLE_FILE}"
grep -Fq 'signingConfigs.getByName("release")' "${GRADLE_FILE}"
echo "patched Android release signing in ${GRADLE_FILE}"
