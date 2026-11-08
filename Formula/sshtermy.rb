# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.253"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.253/sshtermy_darwin_arm64"
      sha256 "60f449783e60e8def704fdf4bee00c7034e96261415702063a9e5b0815d42b6e"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.253/sshtermy_darwin_amd64"
      sha256 "3ced077d5eeee99abf0a4c89e2f3f2e7a2a965584cd66f10e44ee43187e04895"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.253/sshtermy_linux_arm64"
      sha256 "e0a13d5616fda3bfb72ce088861ba850be6a3d49d8af4ea66ccc40beb1b453d1"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.253/sshtermy_linux_amd64"
      sha256 "59fa8b8aca0a260079ebfbd7b933332ce3213b80580fcfb153990993af46fb68"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
