# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.236"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.236/sshtermy_darwin_arm64"
      sha256 "fbceffd03978087e90eb416eca7dde979c44c3899c2b5a1a16484c28310ed947"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.236/sshtermy_darwin_amd64"
      sha256 "bcf1104d535919f453f10a50cfd436c1c7cbdcdf52c2024a40b3632bd34df70c"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.236/sshtermy_linux_arm64"
      sha256 "9e4073d7e57f848a4f6edcfd546897977c448de5a8e6c18a8de5eb04348e1fb7"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.236/sshtermy_linux_amd64"
      sha256 "eafa3256e4e01411ffcb5e219be74533a33579fdaddd2f5866ab8ca20e384be0"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
