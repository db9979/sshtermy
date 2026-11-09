# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.291"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.291/sshtermy_darwin_arm64"
      sha256 "868182df938a28312f206302b41903c885908a74d1d4dc399759bcc33a3a14b9"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.291/sshtermy_darwin_amd64"
      sha256 "87befca5c8880fdad674001e51430857819ef57cd6726d9e7d59d627e80587f5"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.291/sshtermy_linux_arm64"
      sha256 "3097d36afb3b4c85365892cf9524b95f9be89456aa5519c83456a2aa0f893610"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.291/sshtermy_linux_amd64"
      sha256 "c194c80b29dbf10599c1a30d9beba207e8386b37066fadcc2ed32483e68aae83"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
