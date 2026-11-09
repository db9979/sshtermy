# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.296"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.296/sshtermy_darwin_arm64"
      sha256 "27a642a2746f7f04a0d0ad7b80640d4308a24a23cdad1052abc7c94fb1783a99"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.296/sshtermy_darwin_amd64"
      sha256 "a99f26fa5febbb9fe10ff2acf02b65245cfbeb7b59b73160529a93605b8aa361"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.296/sshtermy_linux_arm64"
      sha256 "30068353419ad26514fe43431536857f881fa835cccb42fa759d7af62594e50e"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.296/sshtermy_linux_amd64"
      sha256 "e3934bd2a83bf83b0280f688ef5210ed1e7876e82562decc0e8258e9f7dd5b21"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
