# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.232"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.232/sshtermy_darwin_arm64"
      sha256 "838268050d7f657ebd5b6db0200b9bb9e3502b8422de4718a9eda8249358ec88"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.232/sshtermy_darwin_amd64"
      sha256 "a6a19ea44c86069ae1b96df53d3c4e0317e74c5aea2051f851d20fc66483e4ef"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.232/sshtermy_linux_arm64"
      sha256 "e4eba9729a3f7d913c65dd388d70c7188607329e917bac8942ce9c219724ba2e"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.232/sshtermy_linux_amd64"
      sha256 "dc93479a72915a84a512e80d7adfc544cb27da2106d4455bf386b4a93943877f"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
