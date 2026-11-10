# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.316"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.316/sshtermy_darwin_arm64"
      sha256 "57f07f8787d7f8e516cb6ae68aaab05e99fa5e9830cc96e475d2deaa7f3e9cb1"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.316/sshtermy_darwin_amd64"
      sha256 "6c81db9f194648f4a90468b41b7d297af7f73e07bfc2557a959e1f2c717a50a2"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.316/sshtermy_linux_arm64"
      sha256 "a47b96429299531403052bf57874065e275008c8cff1e7d756a56e81430c4c9a"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.316/sshtermy_linux_amd64"
      sha256 "0ce22603ee2a5e9dd0ba3d3ff66dafcf24c919633e1006ee6d95edea93237e7b"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
