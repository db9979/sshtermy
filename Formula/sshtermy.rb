# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.211"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.211/sshtermy_darwin_arm64"
      sha256 "3ad4e5b35109e90c5e26c42db6835d3dfcdcf5ffad531fc0ac56d80ba81a8d53"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.211/sshtermy_darwin_amd64"
      sha256 "18ba4af38d92d18972772904319654e0f7bd3639ce2e52a1caeadf2f14ed34d2"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.211/sshtermy_linux_arm64"
      sha256 "a85a00aa0b93aba20676bf2832edc931e86cce8fe818d8a37de9e67017f11df3"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.211/sshtermy_linux_amd64"
      sha256 "cbcba43b004dbdea1c54b86c80ba7377cd01e1e5650ea49a20124709c48caad6"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
