# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.326"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.326/sshtermy_darwin_arm64"
      sha256 "e1c6e03b553967679b932da7b1d660d01db4b038a4f523492a959bfcbf5b295e"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.326/sshtermy_darwin_amd64"
      sha256 "8878d1e3c9f22d57c5cccdd530a20d120bab8ad23411622df907bddf90f65222"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.326/sshtermy_linux_arm64"
      sha256 "feb541a57f9c487c4b5bd2f053da5f69f39e3bf4e431a4b68ab610b21de6774e"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.326/sshtermy_linux_amd64"
      sha256 "995d9417b2e880e2b9c43b92e022cc31df126617a4fd756d3fada606267c98de"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
