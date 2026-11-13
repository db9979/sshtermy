# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.398"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.398/sshtermy_darwin_arm64"
      sha256 "aacf7ecf0590f19fe0f0060157090dbd42c05496221606ea45938981f669b0ff"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.398/sshtermy_darwin_amd64"
      sha256 "29f8ce90aa283f852dd08e07493321b6da8160390a077b345a8d774afbf1a10c"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.398/sshtermy_linux_arm64"
      sha256 "c37fc01bf4b2ffbad20c2beac151b49cc917070e06959e8768b197111d263298"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.398/sshtermy_linux_amd64"
      sha256 "17e2e96ddb18370f7463b99cb863fb01ff6138c2beabaf5bf792a91d74d4873e"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
