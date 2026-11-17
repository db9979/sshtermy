# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.424"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.424/sshtermy_darwin_arm64"
      sha256 "2834111d6e5718fa5f376665d27e8bece2f2abc806241ae6c69f6197344f7643"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.424/sshtermy_darwin_amd64"
      sha256 "c81ca668cb3d5d677165be109f7b35272efdddb5ef5337421637d0c2d9ea19a2"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.424/sshtermy_linux_arm64"
      sha256 "3026a5e046acea8bc5266d01c78863e1cf007fa3409e034f2babb4e3cda0383d"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.424/sshtermy_linux_amd64"
      sha256 "13ec3a79f2869a5fa915875fd694398981994f5446d6861e30b83e12d0c63097"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
