# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.274"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.274/sshtermy_darwin_arm64"
      sha256 "782e119cf598bc6d9304cc43318d5df172ca99901ad509a12d3c62d28b1514be"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.274/sshtermy_darwin_amd64"
      sha256 "55318e241814a2ca3c0739742b590d21a09ad6f38e36dd54790f5bbf29ca2779"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.274/sshtermy_linux_arm64"
      sha256 "bce5662415ec795371ea6fb037fa97348171fc6e669885b1ad147bf6b0f469be"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.274/sshtermy_linux_amd64"
      sha256 "68684465f384bb4afefbe24a6211d7e8034f898dbf110cbfa2ce6284acf4013f"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
